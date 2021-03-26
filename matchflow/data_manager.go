package matchflow

import (
	"database/sql"
	"fmt"
	"math/big"
	"sync"

	conflux "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	_ "github.com/go-sql-driver/mysql" // mysql driver
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/shopspring/decimal"
)

// DataManager access matchflow database
type DataManager struct {
	db             *sql.DB
	userName       sync.Map
	userNameCnt    int
	userAccount    sync.Map
	userAccountCnt int
	currency       sync.Map
	product        sync.Map
	order          sync.Map
	orderCnt       int
	dexStartTime   string
}

// User struct for t_user table
type User struct {
	id   uint64
	name string
}

// Trade struct for t_trade table
type Trade struct {
	productID, takerOrderID, makerOrderID   uint64
	price, amount, side, takerFee, makerFee string
}

// TradeDetail struct for transaction replay
type TradeDetail struct {
	userID uint64
	amount decimal.Decimal
}

// Product struct for t_product table
type Product struct {
	baseCurrencyID, quoteCurrencyID uint64
}

// Order struct for t_order table
type Order struct {
	userID                                      uint64
	feeAddress, orderType, price                string
	baseAccountID, quoteAccountID, feeAccountID uint64
}

// Withdraw struct for t_withdraw table
type Withdraw struct {
	userAddress, currency, amount string
}

// Deposit struct for t_deposit table
type Deposit struct {
	userAddress, currency, amount, txHash string
}

// Transfer struct for t_transfer table
type Transfer struct {
	userAddress, currency, recipients string
}

// Account balance
type Account struct {
	account  string
	currency string
	balance  decimal.Decimal
}

const (
	maxCacheSize       = 1000000
	maxRecordsPerQuery = 100
)

// MustGetUserByName get user by conflux address
func (m *DataManager) MustGetUserByName(name string) *User {
	if ret, ok := m.userName.Load(name); ok {
		return ret.(*User)
	}
	rows, err := m.db.Query(`SELECT * FROM t_user WHERE NAME = ?`, name)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	user := User{}
	if rows.Next() {
		if err := rows.Scan(&user.id, &user.name); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Errorf("user %s not found", name))
	}
	m.userName.Store(name, &user)
	m.userNameCnt++
	return &user
}

// MustGetAccountIDByUserID get user account id of specific currency by user id
func (m *DataManager) MustGetAccountIDByUserID(userID uint64, currency string) uint64 {
	var currencyAccountMap *sync.Map
	if ret, ok := m.userAccount.Load(userID); ok {
		currencyAccountMap = ret.(*sync.Map)
		if ret, ok := currencyAccountMap.Load(currency); ok {
			return ret.(uint64)
		}
	} else {
		currencyAccountMap = &sync.Map{}
		m.userAccount.Store(userID, currencyAccountMap)
		m.userAccountCnt++
	}
	rows, err := m.db.Query(`SELECT id FROM t_account WHERE user_id = ? AND currency = ?`, userID, currency)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var accountID uint64
	if rows.Next() {
		if err := rows.Scan(&accountID); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Errorf("user %d account for %s not found", userID, currency))
	}
	currencyAccountMap.Store(currency, accountID)
	return accountID
}

// MustGetAccountID get user account id of specific currency by conflux address
func (m *DataManager) MustGetAccountID(userName string, currency string) uint64 {
	user := m.MustGetUserByName(userName)
	return m.MustGetAccountIDByUserID(user.id, currency)
}

// getTrades get trade with tx_nonce between fromNonce and toNonce with offset
func (m *DataManager) getTrades(fromNonce *big.Int, toNonce *big.Int, offset uint64) []*Trade {
	rows, err := m.db.Query(`
	SELECT product_id, 
		taker_order_id, 
		maker_order_id, 
		price, 
		amount, 
		side, 
		taker_fee,
		maker_fee 
	FROM	t_trade 
	WHERE	status IN ( "onchainsettled", "onchainconfirmed" ) 
			AND tx_nonce BETWEEN ? AND ? 
			AND create_time > ?
	ORDER BY id
	LIMIT  ?, ? `, fromNonce.Int64(), toNonce.Int64(), m.dexStartTime, offset, maxRecordsPerQuery)

	if err != nil {
		panic(err)
	}
	defer rows.Close()

	trades := []*Trade{}
	for rows.Next() {
		trade := Trade{}
		if err := rows.Scan(&trade.productID,
			&trade.takerOrderID,
			&trade.makerOrderID,
			&trade.price,
			&trade.amount,
			&trade.side,
			&trade.takerFee,
			&trade.makerFee); err != nil {
			panic(err)
		}
		trades = append(trades, &trade)
	}
	return trades
}

// GetTrades get trade with tx_nonce between fromNonce and toNonce
func (m *DataManager) GetTrades(fromNonce *big.Int, toNonce *big.Int) []*Trade {
	offset := uint64(0)
	trades := []*Trade{}
	for {
		ret := m.getTrades(fromNonce, toNonce, offset)
		trades = append(trades, ret...)
		offset = offset + uint64(len(ret))
		if len(ret) < maxRecordsPerQuery {
			break
		}
	}
	return trades
}

// getWithdraws get withdraw records with tx_nonce between fromNonce and toNonce with offset
func (m *DataManager) getWithdraws(fromNonce *big.Int, toNonce *big.Int, offset uint64) []*Withdraw {
	rows, err := m.db.Query(`
	SELECT user_address, 
	   currency,
       amount
	FROM   t_withdraw
	WHERE  status IN ( "onchainsettled", "onchainconfirmed" ) 
		AND tx_nonce BETWEEN ? AND ? 
		AND create_time > ?
	ORDER BY id
	LIMIT  ?, ? `, fromNonce.Int64(), toNonce.Int64(), m.dexStartTime, offset, maxRecordsPerQuery)

	if err != nil {
		panic(err)
	}
	defer rows.Close()

	withdraws := []*Withdraw{}
	for rows.Next() {
		withdraw := Withdraw{}
		if err := rows.Scan(&withdraw.userAddress,
			&withdraw.currency,
			&withdraw.amount); err != nil {
			panic(err)
		}
		withdraws = append(withdraws, &withdraw)
	}
	return withdraws
}

// GetWithdraws get withdraw record with tx_nonce between fromNonce and toNonce
func (m *DataManager) GetWithdraws(fromNonce *big.Int, toNonce *big.Int) []*Withdraw {
	offset := uint64(0)
	withdraws := []*Withdraw{}
	for {
		ret := m.getWithdraws(fromNonce, toNonce, offset)
		withdraws = append(withdraws, ret...)
		offset = offset + uint64(len(ret))
		if len(ret) < maxRecordsPerQuery {
			break
		}
	}
	return withdraws
}

// GetCurrencyName get currency name by currency id
func (m *DataManager) GetCurrencyName(id uint64) string {
	if ret, ok := m.currency.Load(id); ok {
		return ret.(string)
	}
	rows, err := m.db.Query(`SELECT name FROM t_currency WHERE id = ?`, id)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var name string
	if rows.Next() {
		if err := rows.Scan(&name); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Errorf("currency %d not found", id))
	}
	m.currency.Store(id, name)
	return name
}

// GetProduct get product by id
func (m *DataManager) GetProduct(id uint64) *Product {
	if ret, ok := m.product.Load(id); ok {
		return ret.(*Product)
	}
	rows, err := m.db.Query(`
	SELECT base_currency_id,
		quote_currency_id 
	FROM t_product
	WHERE id = ?`, id)

	if err != nil {
		panic(err)
	}
	defer rows.Close()

	product := Product{}
	if rows.Next() {
		if err := rows.Scan(&product.baseCurrencyID, &product.quoteCurrencyID); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Errorf("product %d not found", id))
	}
	m.product.Store(id, &product)
	return &product
}

// GetOrder get order by id
func (m *DataManager) GetOrder(id uint64) *Order {
	if ret, ok := m.order.Load(id); ok {
		return ret.(*Order)
	}
	rows, err := m.db.Query(`
	SELECT user_id,
		fee_address,
		type,
		price
	FROM t_order
	WHERE id = ?`, id)

	if err != nil {
		panic(err)
	}
	defer rows.Close()

	order := Order{}
	if rows.Next() {
		if err := rows.Scan(&order.userID, &order.feeAddress, &order.orderType, &order.price); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Errorf("order %d not found", id))
	}
	m.product.Store(id, &order)
	m.orderCnt++
	return &order
}

func (m *DataManager) getDepositCnt() uint64 {
	rows, err := m.db.Query(`
		SELECT count(*) 
		FROM t_deposit 
		WHERE create_time > ?`, m.dexStartTime)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var total uint64
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Errorf("failed to get count of t_deposit"))
	}
	return total
}

func (m *DataManager) getDeposits(offset uint64, cnt int) []*Deposit {
	rows, err := m.db.Query(`
	SELECT user_address,
		currency,
		amount,
		tx_hash
	FROM t_deposit
	WHERE create_time > ?
	ORDER BY id
	LIMIT ?, ?`, m.dexStartTime, offset, cnt)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	deposits := []*Deposit{}
	for rows.Next() {
		deposit := Deposit{}
		if err := rows.Scan(&deposit.userAddress,
			&deposit.currency,
			&deposit.amount,
			&deposit.txHash); err != nil {
			panic(err)
		}
		deposits = append(deposits, &deposit)
	}
	return deposits
}

func (m *DataManager) getDepositOffset(epoch, total uint64, client *conflux.Client) uint64 {
	// get the minimal offset index which tx epoch number >= given epoch
	l, r := uint64(0), total-1
	for l <= r {
		mid := (l + r) >> 1
		deposit := (m.getDeposits(mid, 1))[0]
		tx, err := client.GetTransactionByHash(types.Hash(deposit.txHash))
		if err != nil {
			panic(err)
		}
		block, err := client.GetBlockByHash(*tx.BlockHash)
		if err != nil {
			panic(err)
		}
		midEpoch := block.EpochNumber.ToInt().Uint64()
		if midEpoch < epoch {
			l = mid + 1
		} else {
			r = mid - 1
		}
	}
	return r + 1
}

// GetDeposits get deposit records between fromEpoch and toEpoch
func (m *DataManager) GetDeposits(fromEpoch, toEpoch *big.Int, client *conflux.Client) []*Deposit {
	deposits := []*Deposit{}
	total := m.getDepositCnt()
	if total == 0 {
		return deposits
	}
	startOffset, endOffset := m.getDepositOffset(fromEpoch.Uint64(), total, client), m.getDepositOffset(toEpoch.Uint64()+uint64(1), total, client)
	endOffset--
	if startOffset > endOffset {
		return deposits
	}
	i := startOffset
	for i <= endOffset {
		cnt := maxRecordsPerQuery
		if int(endOffset-i+1) < cnt {
			cnt = int(endOffset - i + 1)
		}
		ret := m.getDeposits(i, cnt)
		deposits = append(deposits, ret...)
		i = i + uint64(len(ret))
	}
	return deposits
}

// getTransfers get transfer records with tx_nonce between fromNonce and toNonce with offset
func (m *DataManager) getTransfers(fromNonce *big.Int, toNonce *big.Int, offset uint64) []*Transfer {
	rows, err := m.db.Query(`
	SELECT user_address, 
	   currency,
       recipients
	FROM   t_transfer
	WHERE  status IN ( "onchainsettled", "onchainconfirmed" ) 
		AND tx_nonce BETWEEN ? AND ? 
		AND create_time > ?
	ORDER BY id
	LIMIT  ?, ? `, fromNonce.Int64(), toNonce.Int64(), m.dexStartTime, offset, maxRecordsPerQuery)

	if err != nil {
		panic(err)
	}
	defer rows.Close()

	transfers := []*Transfer{}
	for rows.Next() {
		transfer := Transfer{}
		if err := rows.Scan(&transfer.userAddress,
			&transfer.currency,
			&transfer.recipients); err != nil {
			panic(err)
		}
		transfers = append(transfers, &transfer)
	}
	return transfers
}

// GetTransfers get transfer records between fromNonce and toNonce
func (m *DataManager) GetTransfers(fromNonce, toNonce *big.Int) []*Transfer {
	offset := uint64(0)
	transfers := []*Transfer{}
	for {
		ret := m.getTransfers(fromNonce, toNonce, offset)
		transfers = append(transfers, ret...)
		offset = offset + uint64(len(ret))
		if len(ret) < maxRecordsPerQuery {
			break
		}
	}
	return transfers
}

// CleanCache clean the cache map if it size exceeds a constant
func (m *DataManager) CleanCache() {
	if m.userNameCnt > maxCacheSize {
		m.userName = sync.Map{}
		m.userNameCnt = 0
	}
	if m.userAccountCnt > maxCacheSize {
		m.userAccount = sync.Map{}
		m.userAccountCnt = 0
	}
	if m.orderCnt > maxCacheSize {
		m.order = sync.Map{}
		m.orderCnt = 0
	}
}

// NewDataManager create new datamanager instance
func NewDataManager(dexDbUser, dbAddress, dbPass string, dexStartTime string) *DataManager {
	dbDriver := "mysql"
	dbUser := dexDbUser
	dbName := "conflux_dex"
	decrypted := common.AesDecrypt(dbPass, common.AesSecret)
	db, err := sql.Open(dbDriver, dbUser+":"+decrypted+"@"+dbAddress+"/"+dbName)
	if err != nil {
		panic(err.Error())
	}
	return &DataManager{
		db:             db,
		userName:       sync.Map{},
		userNameCnt:    0,
		userAccount:    sync.Map{},
		userAccountCnt: 0,
		currency:       sync.Map{},
		product:        sync.Map{},
		order:          sync.Map{},
		orderCnt:       0,
		dexStartTime:   dexStartTime,
	}
}
