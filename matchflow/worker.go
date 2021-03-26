package matchflow

import (
	"encoding/csv"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	conflux "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/Conflux-Chain/go-conflux-sdk/types/cfxaddress"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/shopspring/decimal"
)

const (
	ten18           = "1000000000000000000" // 1e18
	module          = "matchflow"           // module name
	maxGoroutineNum = 5                     // max goroutine number
)

// Worker auditor
type Worker struct {
	lastFullAuditEpoch    *big.Int
	lastPartialAuditEpoch *big.Int
	matchflowClient       *common.Client
	cfxClient             *conflux.Client
	db                    *DataManager
	assetsMap             map[string]*common.Contract
	pausable              bool
}

// BalanceChange user with `accountID` has `amount` change of balance
type BalanceChange struct {
	accountID uint64
	amount    *big.Int
}

// DataToAddress convert a hex string returned in topics of log to conflux address
func DataToAddress(data string) string {
	return "0x" + data[26:]
}

// NewWorker create a new worker
func NewWorker(matchflowClient *common.Client, cfxClient *conflux.Client, assetsMap map[string]*common.Contract, config *common.MatchflowConfig) *Worker {
	w := &Worker{
		matchflowClient: matchflowClient,
		cfxClient:       cfxClient,
		db:              NewDataManager(config.DbUser, config.DbAddress, config.DbPass, config.DexStartTime),
		assetsMap:       assetsMap,
		pausable:        config.Pausable,
	}
	return w
}

func (w *Worker) getOnchainBalanceChange(userAddress, currency string, fromEpoch, toEpoch *big.Int, ch chan *BalanceChange) {
	accountID := w.db.MustGetAccountID(userAddress, currency)
	fromBalance := w.assetsMap[currency].MustGetBalanceOf(userAddress, types.NewEpochNumberBig(big.NewInt(0).Sub(fromEpoch, big.NewInt(1))))
	toBalance := w.assetsMap[currency].MustGetBalanceOf(userAddress, types.NewEpochNumberBig(toEpoch))
	ch <- &BalanceChange{
		accountID: accountID,
		amount:    big.NewInt(0).Sub(toBalance, fromBalance),
	}
}

func (w *Worker) assetSync(asset string, fromEpoch *big.Int, toEpoch *big.Int, ch chan map[uint64]*big.Int, isFull bool) {
	addresses := make(map[string]bool)
	if !isFull {
		// just audit balance of addresses appeared in events
		for i := big.NewInt(0).Set(fromEpoch); toEpoch.Cmp(i) >= 0; i.Add(i, big.NewInt(1)) {
			logs, err := w.cfxClient.GetLogs(types.LogFilter{
				FromEpoch: types.NewEpochNumberBig(i),
				ToEpoch:   types.NewEpochNumberBig(i),
				Address:   []types.Address{*w.assetsMap[asset].Contract.Address},
			})
			if err != nil {
				panic(err)
			}
			for _, log := range logs {
				switch log.Topics[0] {
				case common.EventHashTransfer:
					addresses[DataToAddress(log.Topics[1].String())] = true
					addresses[DataToAddress(log.Topics[2].String())] = true
				case common.EventHashDeposit:
				case common.EventHashWithdraw, common.EventHashWrite:
				default:
				}
			}
		}
	} else {
		// audit all addresses
		epoch := types.NewEpochNumberBig(toEpoch)
		ret := w.assetsMap[asset].ListAllAccounts(epoch)
		for _, address := range ret {
			addresses[address] = true
		}
	}
	delete(addresses, common.ZeroAddress)
	result := make(map[uint64]*big.Int)
	balanceChangeCh := make(chan *BalanceChange)
	k := 0
	for address := range addresses {
		if k == maxGoroutineNum {
			for i := 0; i < k; i++ {
				ans := <-balanceChangeCh
				result[ans.accountID] = ans.amount
			}
			k = 0
		}
		go w.getOnchainBalanceChange(address, asset, fromEpoch, toEpoch, balanceChangeCh)
		k++
	}
	for i := 0; i < k; i++ {
		ans := <-balanceChangeCh
		result[ans.accountID] = ans.amount
	}
	ch <- result
}

func (w *Worker) onchainSync(fromEpoch *big.Int, toEpoch *big.Int, ch chan map[uint64]*big.Int, isFull bool) {
	assetSyncCh := make(chan map[uint64]*big.Int)
	for k := range w.assetsMap {
		go w.assetSync(k, fromEpoch, toEpoch, assetSyncCh, isFull)
	}
	result := make(map[uint64]*big.Int)
	for i := 0; i < len(w.assetsMap); i++ {
		assetResult := <-assetSyncCh
		for k, v := range assetResult {
			result[k] = v
		}
	}
	ch <- result
}

func parseFloat(x string) decimal.Decimal {
	ans, err := decimal.NewFromString(x)
	if err != nil {
		panic(err)
	}
	return ans
}

func (w *Worker) parseTrade(trade *Trade, ch chan []*TradeDetail) {
	product := w.db.GetProduct(trade.productID)

	baseCurrencyName := w.db.GetCurrencyName(product.baseCurrencyID)
	quoteCurrencyName := w.db.GetCurrencyName(product.quoteCurrencyID)

	takerOrder := w.db.GetOrder(trade.takerOrderID)
	makerOrder := w.db.GetOrder(trade.makerOrderID)

	takerOrder.baseAccountID = w.db.MustGetAccountIDByUserID(takerOrder.userID, baseCurrencyName)
	takerOrder.quoteAccountID = w.db.MustGetAccountIDByUserID(takerOrder.userID, quoteCurrencyName)
	makerOrder.baseAccountID = w.db.MustGetAccountIDByUserID(makerOrder.userID, baseCurrencyName)
	makerOrder.quoteAccountID = w.db.MustGetAccountIDByUserID(makerOrder.userID, quoteCurrencyName)

	details := []*TradeDetail{}
	tradeAmount := parseFloat(trade.amount)
	tradePrice := parseFloat(trade.price)
	tradeFunds := parseFloat(trade.amount)
	tradeFunds = common.Mul(tradeFunds, tradePrice)
	takerFee := parseFloat(trade.takerFee)
	makerFee := parseFloat(trade.makerFee)

	if trade.side == "Buy" {
		takerOrder.feeAccountID = w.db.MustGetAccountID(takerOrder.feeAddress, baseCurrencyName)
		makerOrder.feeAccountID = w.db.MustGetAccountID(makerOrder.feeAddress, quoteCurrencyName)
		/*
			refundAmount := parseFloat("0")
			if takerOrder.orderType == "Limit" {
				refundAmount.Mul(parseFloat(takerOrder.price), tradeAmount)
				refundAmount.Sub(refundAmount, tradeFunds)
			}
		*/

		// taker side
		details = append(details, &TradeDetail{
			userID: takerOrder.baseAccountID,
			amount: tradeAmount.Sub(takerFee),
		})
		details = append(details, &TradeDetail{
			userID: takerOrder.feeAccountID,
			amount: takerFee,
		})
		details = append(details, &TradeDetail{
			userID: takerOrder.quoteAccountID,
			amount: tradeFunds.Neg(),
		})
		// maker side
		details = append(details, &TradeDetail{
			userID: makerOrder.baseAccountID,
			amount: tradeAmount.Neg(),
		})
		details = append(details, &TradeDetail{
			userID: makerOrder.quoteAccountID,
			amount: tradeFunds.Sub(makerFee),
		})
		details = append(details, &TradeDetail{
			userID: makerOrder.feeAccountID,
			amount: makerFee,
		})
	} else {
		takerOrder.feeAccountID = w.db.MustGetAccountID(takerOrder.feeAddress, quoteCurrencyName)
		makerOrder.feeAccountID = w.db.MustGetAccountID(makerOrder.feeAddress, baseCurrencyName)

		// taker side
		details = append(details, &TradeDetail{
			userID: takerOrder.baseAccountID,
			amount: tradeAmount.Neg(),
		})
		details = append(details, &TradeDetail{
			userID: takerOrder.quoteAccountID,
			amount: tradeFunds.Sub(takerFee),
		})
		details = append(details, &TradeDetail{
			userID: takerOrder.feeAccountID,
			amount: takerFee,
		})
		// maker side
		details = append(details, &TradeDetail{
			userID: makerOrder.baseAccountID,
			amount: tradeAmount.Sub(makerFee),
		})
		details = append(details, &TradeDetail{
			userID: makerOrder.feeAccountID,
			amount: makerFee,
		})
		details = append(details, &TradeDetail{
			userID: makerOrder.quoteAccountID,
			amount: tradeFunds.Neg(),
		})
	}
	ch <- details
}

func (w *Worker) parseWithdraw(withdraw *Withdraw, ch chan []*TradeDetail) {
	details := []*TradeDetail{}
	details = append(details, &TradeDetail{
		userID: w.db.MustGetAccountID(withdraw.userAddress, withdraw.currency),
		amount: parseFloat(withdraw.amount).Neg(),
	})
	ch <- details
}

func (w *Worker) parseDeposit(deposit *Deposit, ch chan []*TradeDetail) {
	details := []*TradeDetail{}
	details = append(details, &TradeDetail{
		userID: w.db.MustGetAccountID(deposit.userAddress, deposit.currency),
		amount: parseFloat(deposit.amount),
	})
	ch <- details
}

func (w *Worker) parseTransfer(transfer *Transfer, ch chan []*TradeDetail) {
	details := []*TradeDetail{}
	pairs := strings.Split(strings.Trim(transfer.recipients, "{}"), ",")
	senderAccountID := w.db.MustGetAccountID(transfer.userAddress, transfer.currency)
	for _, pair := range pairs {
		elements := strings.Split(pair, ":")
		recipientAddress := strings.Trim(elements[0], `"`)
		recipientAccountID := w.db.MustGetAccountID(recipientAddress, transfer.currency)
		recipientAmount := parseFloat(elements[1])
		details = append(details, &TradeDetail{
			userID: senderAccountID,
			amount: recipientAmount.Neg(),
		})
		details = append(details, &TradeDetail{
			userID: recipientAccountID,
			amount: recipientAmount,
		})
	}
	ch <- details
}

func (w *Worker) offchainReplay(fromEpoch *big.Int, toEpoch *big.Int, ch chan map[uint64]*big.Int) {
	// get nonce range
	fromNonceWrap, err := w.cfxClient.GetNextNonce(cfxaddress.MustNewFromHex(common.DexAdmin, common.GetNetworkId()), types.NewEpochNumberBig(big.NewInt(0).Sub(fromEpoch, big.NewInt(1))))
	if err != nil {
		panic(err)
	}
	toNonceWrap, err := w.cfxClient.GetNextNonce(cfxaddress.MustNewFromHex(common.DexAdmin, common.GetNetworkId()), types.NewEpochNumberBig(toEpoch))
	if err != nil {
		panic(err)
	}
	fromNonce := fromNonceWrap.ToInt()
	toNonce := toNonceWrap.ToInt()
	toNonce.Sub(toNonce, big.NewInt(1))
	logger.Infof("nonce from %s to %s", fromNonce, toNonce)

	tradeDetailCh := make(chan []*TradeDetail)
	details := []*TradeDetail{}

	k := 0
	if fromNonce.Cmp(toNonce) <= 0 {
		// get trades
		trades := w.db.GetTrades(fromNonce, toNonce)
		logger.Infof("trade amount: %d", len(trades))
		for _, trade := range trades {
			if k == maxGoroutineNum {
				for i := 0; i < k; i++ {
					ans := <-tradeDetailCh
					details = append(details, ans...)
				}
				k = 0
			}
			go w.parseTrade(trade, tradeDetailCh)
			k++
		}
		for i := 0; i < k; i++ {
			ans := <-tradeDetailCh
			details = append(details, ans...)
		}

		// get withdraw records
		withdraws := w.db.GetWithdraws(fromNonce, toNonce)
		k = 0
		for _, withdraw := range withdraws {
			if k == maxGoroutineNum {
				for i := 0; i < k; i++ {
					ans := <-tradeDetailCh
					details = append(details, ans...)
				}
				k = 0
			}
			go w.parseWithdraw(withdraw, tradeDetailCh)
			k++
		}
		for i := 0; i < k; i++ {
			ans := <-tradeDetailCh
			details = append(details, ans...)
		}

		// get transfer records
		k = 0
		transfers := w.db.GetTransfers(fromNonce, toNonce)
		for _, transfer := range transfers {
			if k == maxGoroutineNum {
				for i := 0; i < k; i++ {
					ans := <-tradeDetailCh
					details = append(details, ans...)
				}
				k = 0
			}
			go w.parseTransfer(transfer, tradeDetailCh)
			k++
		}
		for i := 0; i < k; i++ {
			ans := <-tradeDetailCh
			details = append(details, ans...)
		}
	}

	// get deposit records
	k = 0
	deposits := w.db.GetDeposits(fromEpoch, toEpoch, w.cfxClient)
	for _, deposit := range deposits {
		if k == maxGoroutineNum {
			for i := 0; i < k; i++ {
				ans := <-tradeDetailCh
				details = append(details, ans...)
			}
			k = 0
		}
		go w.parseDeposit(deposit, tradeDetailCh)
		k++
	}
	for i := 0; i < k; i++ {
		ans := <-tradeDetailCh
		details = append(details, ans...)
	}

	// conclude
	result := make(map[uint64]*big.Int)
	for _, detail := range details {
		detailAmount := common.Mul(detail.amount, parseFloat(ten18))
		detailAmountInt := detailAmount.BigInt()
		if amount, ok := result[detail.userID]; ok {
			amount.Add(amount, detailAmountInt)
		} else {
			result[detail.userID] = detailAmountInt
		}
	}
	ch <- result
}

func filterZeroValue(m map[uint64]*big.Int) {
	toRemove := []uint64{}
	for k, v := range m {
		if v.Sign() == 0 {
			toRemove = append(toRemove, k)
		}
	}
	for _, k := range toRemove {
		delete(m, k)
	}
}

func (w *Worker) audit(fromEpoch *big.Int, toEpoch *big.Int, isFull bool) {
	logger.Infof("audit from %s to %s, isFull: %t\n", fromEpoch, toEpoch, isFull)

	onchainSyncCh, offchainReplayCh := make(chan map[uint64]*big.Int), make(chan map[uint64]*big.Int)
	go w.onchainSync(fromEpoch, toEpoch, onchainSyncCh, isFull)
	go w.offchainReplay(fromEpoch, toEpoch, offchainReplayCh)
	onchainResult, offchainResult := <-onchainSyncCh, <-offchainReplayCh
	logger.Infof("onchain #account with balance change: %v", len(onchainResult))
	logger.Infof("offchain #account with balance change: %v", len(offchainResult))
	filterZeroValue(onchainResult)
	filterZeroValue(offchainResult)
	logger.Infof("onchain #account with balance change abs > 0: %v", len(onchainResult))
	logger.Infof("offchain #account with balance change abs > 0: %v", len(offchainResult))
	if len(onchainResult) != len(offchainResult) {
		err := fmt.Sprintf("list of account with balance change of onchain(%d) and offchain(%d) are different for epoch %s to %s!",
			len(onchainResult), len(offchainResult), fromEpoch, toEpoch)
		logger.Errorf("%v\n", onchainResult)
		logger.Errorf("%v\n", offchainResult)
		logger.Errorf(err)
		common.Alert(module, err)
		if w.pausable {
			common.AlertMatchflow()
		}
	}
	for accountID, onchainAmount := range onchainResult {
		if offchainAmount, ok := offchainResult[accountID]; ok {
			if onchainAmount.Cmp(offchainAmount) != 0 {
				err := fmt.Sprintf("account with ID %d onchain balance change is different with offchain. onchain: %s, offchain: %s",
					accountID, onchainAmount, offchainAmount)
				logger.Errorf(err)
				common.Alert(module, err)
				if w.pausable {
					common.AlertMatchflow()
				}
			}
		} else {
			err := fmt.Sprintf("account with ID %d onchain balance changed but offchain didn't!", accountID)
			logger.Errorf(err)
			common.Alert(module, err)
			if w.pausable {
				common.AlertMatchflow()
			}
		}
	}
}

func parseEpoch(epoch string, bestEpoch *big.Int) *big.Int {
	if epochNum, ok := new(big.Int).SetString(epoch, 10); ok {
		if epochNum.Sign() < 0 {
			epochNum.Add(bestEpoch, epochNum)
		}
		return epochNum
	}
	panic("invalid epoch number format")
}

func listAllAccounts(c *common.Contract, epoch *types.Epoch, ch chan []string) {
	ch <- c.ListAllAccounts(epoch)
}

func (w *Worker) runFetcher(accounts map[string]bool, accountCh chan *Account) {
	for account := range accounts {
		balances, err := w.matchflowClient.GetBalance(account)
		if err != nil {
			logger.Errorf("Non-exisitng Account: %s", account)
		}

		for _, balance := range balances {
			b, err := decimal.NewFromString(balance.Balance)
			if err != nil {
				panic(err)
			}
			accountCh <- &Account{account, balance.Currency, b}
		}
	}

	close(accountCh)
}

var pow10_18 = new(big.Float).SetPrec(200).SetInt(big.NewInt(1000000000000000000))

func (w *Worker) auditAccountBalance(name string, account string, balance decimal.Decimal) []string {
	if name == "EOS" || name == "CNY" {
		return nil
	}
	balance = common.Mul(balance, parseFloat(ten18))
	offchainBalance := balance.BigInt()

	onchainBalance, err := w.assetsMap[name].BalanceOf(account)
	if err != nil {
		panic(err)
	}

	if offchainBalance.Cmp(onchainBalance) != 0 {
		logger.Errorf("balance mismatch: %s %s %s %s %d", account, name, offchainBalance.Text(10), onchainBalance.Text(10), offchainBalance.Cmp(onchainBalance))
		return []string{account, name, offchainBalance.Text(10), onchainBalance.Text(10)}
	}
	return nil
}

func (w *Worker) initialAudit() bool {
	logger.Infof("start initial balance audit..")
	bestEpochWrap, err := w.cfxClient.GetEpochNumber(types.EpochLatestState)
	if err != nil {
		panic(err)
	}
	bestEpoch := bestEpochWrap.ToInt()
	logger.Infof("best epoch: %s", bestEpoch)
	bestEpoch.Sub(bestEpoch, common.NumEpochsConfirmed)

	file, err := os.Create("matchflow_initial_audit_result.csv")
	if err != nil {
		panic(err)
	}
	writer := csv.NewWriter(file)

	addressCh := make(chan []string)
	for _, c := range w.assetsMap {
		go listAllAccounts(c, types.NewEpochNumberBig(bestEpoch), addressCh)
	}
	addressMap := make(map[string]bool)
	for range w.assetsMap {
		ret := <-addressCh
		for _, address := range ret {
			addressMap[address] = true
		}
	}
	logger.Infof("total %d addresses found for audit.", len(addressMap))
	accountCh := make(chan *Account)
	go w.runFetcher(addressMap, accountCh)

	var records [][]string
	hasErr := false
	for {
		account, more := <-accountCh
		if more {
			err := w.auditAccountBalance(account.currency, account.account, account.balance)
			if err != nil {
				records = append(records, err)
				hasErr = true
			}
		} else {
			logger.Infof("Initial audit done.")
			break
		}
	}
	writer.WriteAll(records)
	file.Close()
	return hasErr
}

// Start audit dex users' balance periodically
func (w *Worker) Start(config *common.MatchflowConfig) {
	if config.InitialAudit {
		if w.initialAudit() {
			logger.Infof("There are errors in initial balance audit, terminate.")
			os.Exit(1)
		}
	}

	bestEpochWrap, err := w.cfxClient.GetEpochNumber(types.EpochLatestState)
	if err != nil {
		panic(err)
	}
	bestEpoch := bestEpochWrap.ToInt()
	bestEpoch.Sub(bestEpoch, common.NumEpochsConfirmed)
	logger.Infof("best epoch: %s", bestEpoch)
	w.lastPartialAuditEpoch = parseEpoch(config.PartialEpoch, bestEpoch)
	w.lastFullAuditEpoch = parseEpoch(config.FullEpoch, bestEpoch)

	for {
		bestEpochWrap, err := w.cfxClient.GetEpochNumber(types.EpochLatestState)
		if err != nil {
			panic(err)
		}
		bestEpoch := bestEpochWrap.ToInt()
		bestEpoch.Sub(bestEpoch, common.NumEpochsConfirmed)
		if new(big.Int).Sub(bestEpoch, w.lastFullAuditEpoch).Cmp(big.NewInt(10000)) >= 0 {
			// do fully audit once per 10000 epoch
			w.audit(w.lastFullAuditEpoch, bestEpoch, true)
			w.lastFullAuditEpoch = bestEpoch
		} else {
			// do partial audit
			for ; bestEpoch.Cmp(w.lastPartialAuditEpoch) > 0; w.lastPartialAuditEpoch.Add(w.lastPartialAuditEpoch, big.NewInt(1)) {
				w.audit(w.lastPartialAuditEpoch, w.lastPartialAuditEpoch, false)
			}
		}
		w.db.CleanCache()
		time.Sleep(5 * time.Second)
	}
}
