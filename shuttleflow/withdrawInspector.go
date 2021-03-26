package shuttleflow

import (
	//"os"
	"crypto/md5"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"
	//"encoding/base64"
	"encoding/hex"

	conflux "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/Conflux-Chain/go-conflux-sdk/types/cfxaddress"
	//"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/open-dex/conflux-dex-audit/common"
	//"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	//"github.com/ethereum/go-ethereum/ethclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var burntEventHash ethCommon.Hash
var opReturnMap map[string]int

// WithdrawInspector auditor
type WithdrawInspector struct {
	Client     *conflux.Client
	Timeout    time.Duration
	Erc777Map  map[string]*common.Contract
	BTCWallet  string
	ETHFactory *bind.BoundContract
	Cb         *Config
}

// Start up an auditor
func (w *WithdrawInspector) Start(wg *sync.WaitGroup, vp *viper.Viper) {
	burntEventHash = crypto.Keccak256Hash([]byte("Burnt(uint256,string,address)"))
	opReturnMap = make(map[string]int)

	wg.Add(2)
	go w.runCFXListener(wg, vp)
	go w.runBTCListener(wg, vp)
}

func (w *WithdrawInspector) runCFXListener(wg *sync.WaitGroup, vp *viper.Viper) {
	defer wg.Done()

	erc777Values := make([]types.Address, 0)
	for _, v := range w.Erc777Map {
		erc777Values = append(erc777Values, cfxaddress.MustNewFromHex(v.Address(), common.GetNetworkId()))
	}

	topics := [][]types.Hash{{types.Hash(burntEventHash.Hex())}}

	var currentBlock big.Int
	currentBlock.SetString(w.Cb.CFXInitialBlock, 10)
	for {
		w.waitForEpochConfirmed(&currentBlock)

		logs, err := GetCFXEvents(w.Client, &currentBlock, erc777Values, topics)
		if err != nil {
			logger.WithError(err).Warn("Error getting CFX withdraw events for epoch ", currentBlock)
			time.Sleep(time.Second)
			continue
		}

		for _, log := range logs {
			_, to, amount := w.decodeBurntEvent(&log)

			switch log.Address.String() {
			case w.Erc777Map["ETH"].Address():
				if amount.Cmp(big.NewInt(vp.GetInt64("eth.minimalWithdraw"))) != -1 {
					w.checkETHCompletion("ETH", log.TransactionHash.String())
				}
			case w.Erc777Map["USDT"].Address():
				if amount.Cmp(big.NewInt(vp.GetInt64("usdt.minimalWithdraw"))) != -1 {
					w.checkETHCompletion("USDT", log.TransactionHash.String())
				}
			case w.Erc777Map["BTC"].Address():
				if amount.Cmp(big.NewInt(vp.GetInt64("btc.minimalWithdraw"))) != -1 {
					str := "burn@" + w.BTCWallet + "@" + to + "@0x" + (new(big.Int).Div(amount, pow10_10)).Text(16) + "#" + log.TransactionHash.String()
					md5Hash := getMD5Hash(str)
					w.checkBTCCompletion("BTC", log.TransactionHash.String(), md5Hash)
				}
			default:
				panic("unrecognized assets")
			}
		}

		currentBlock.Add(&currentBlock, common.Big1)
	}
}

func (w *WithdrawInspector) runBTCListener(wg *sync.WaitGroup, vp *viper.Viper) {
	defer wg.Done()

	prevHeight := GetBTCCurrentHeight() - w.Cb.BTCMinus
	for {
		currentHeight := GetBTCCurrentHeight() + 1
		for prevHeight == currentHeight {
			time.Sleep(90 * time.Second)
			currentHeight = GetBTCCurrentHeight() + 1
		}

		logger.WithFields(logrus.Fields{
			"submodule": "withdraw",
		}).Debug("BTC ", prevHeight, " ", currentHeight)

		txs := GetBTCEvents(w.BTCWallet, big.NewInt(prevHeight), big.NewInt(currentHeight))
		for _, tx := range txs {
			if tx.Inputs[0].Addresses[0] == w.BTCWallet {
				/*totalValue := 0
				for _, input := range tx.Inputs {
					totalValue = totalValue + input.OutputValue
				}

				if totalValue >= vp.GetInt("btc.minimalWithdraw") {*/
				for _, output := range tx.Outputs {
					if output.DataString != "" {
						opReturnData := strings.Split(output.DataString, " ")
						md5 := strings.Split(opReturnData[0], "_")[0]

						total, err := strconv.Atoi(opReturnData[1])
						if err != nil {
							logger.WithFields(logrus.Fields{
								"submodule": "withdraw",
							}).Warn("Atoi")
						}

						_, ok := opReturnMap[md5]
						if !ok {
							opReturnMap[md5] = total - 1
						} else {
							opReturnMap[md5] = opReturnMap[md5] - 1
						}
					}
					//}
				}
			}
		}

		prevHeight = currentHeight

		time.Sleep(60 * time.Second)
	}
}

func (w *WithdrawInspector) waitForEpochConfirmed(epoch *big.Int) {
	for {
		current, err := w.Client.GetEpochNumber(types.EpochLatestState)
		if err != nil {
			logger.WithError(err).Warn("failed to get epoch number from full node")
			time.Sleep(time.Second)
			continue
		}

		confirmedEpoch := new(big.Int).Sub(current.ToInt(), common.NumEpochsConfirmed)

		if epoch.Cmp(confirmedEpoch) > 0 {
			logger.WithFields(logrus.Fields{
				"epochAudit":       epoch,
				"epochConfirmed":   confirmedEpoch,
				"epochLatestState": current,
			}).Trace("audit epoch is not confirmed yet")
			time.Sleep(time.Second)
			continue
		}

		break
	}
}

func (w *WithdrawInspector) decodeBurntEvent(log *types.Log) (string, string, *big.Int) {
	var LogBurnt struct {
		Amount      *big.Int
		ToAddress   string
		FromAddress ethCommon.Address
	}

	err := w.Erc777Map["USDT"].Contract.DecodeEvent(&LogBurnt, "Burnt", *log)
	if err != nil {
		panic(err)
	}
	return LogBurnt.FromAddress.String(), LogBurnt.ToAddress, LogBurnt.Amount
}

func (w *WithdrawInspector) checkETHCompletion(name string, txHash string) {
	flag := MustGetBurnedTx(w.ETHFactory, txHash, &bind.CallOpts{})
	if flag {
		logger.WithFields(logrus.Fields{
			"submodule": "withdraw",
			"name":      name,
		}).Info("Transaction Exists ", txHash)
	} else {
		logger.WithFields(logrus.Fields{
			"submodule": "withdraw",
			"name":      name,
		}).Info("Transaction Waiting ", txHash)
		go w.waitForETHCompletion(name, txHash)
	}
}

func (w *WithdrawInspector) checkBTCCompletion(name string, txHash string, md5Hash string) {
	val, flag := opReturnMap[md5Hash]
	if flag && val == 0 {
		logger.WithFields(logrus.Fields{
			"submodule": "withdraw",
			"name":      name,
		}).Info("Transaction Exists ", txHash, " md5Hash ", md5Hash)
	} else {
		logger.WithFields(logrus.Fields{
			"submodule": "withdraw",
			"name":      name,
		}).Info("Transaction Waiting ", txHash, " md5Hash ", md5Hash)
		go w.waitForBTCCompletion(name, txHash, md5Hash)
	}
}

func (w *WithdrawInspector) waitForETHCompletion(name string, txHash string) {
	timeout := time.After(w.Timeout)
	tick := time.Tick(60 * time.Second)

	for {
		select {
		case <-timeout:
			logger.WithFields(logrus.Fields{
				"submodule": "withdraw",
				"name":      name,
			}).Warn("Transaction Timeout ", txHash)
			err := fmt.Sprintf("%s withdraw %s timeout: %s", name, w.Timeout.String(), txHash)
			common.Alert(module, err)
			return

		case <-tick:
			flag := MustGetBurnedTx(w.ETHFactory, txHash, &bind.CallOpts{})
			if flag {
				return
			}
			logger.WithFields(logrus.Fields{
				"submodule": "withdraw",
				"name":      name,
			}).Info("Transaction Waiting ", txHash)
		}
	}
}

func (w *WithdrawInspector) waitForBTCCompletion(name string, txHash string, md5Hash string) {
	timeout := time.After(w.Timeout)
	tick := time.Tick(60 * time.Second)

	for {
		select {
		case <-timeout:
			logger.WithFields(logrus.Fields{
				"submodule": "withdraw",
				"name":      name,
			}).Warn("Transaction Timeout ", txHash, " md5Hash ", md5Hash)

			err := fmt.Sprintf("%s withdraw %s timeout: %s md5Hash: %s", name, w.Timeout.String(), txHash, md5Hash)
			common.Alert(module, err)
			return

		case <-tick:
			val, flag := opReturnMap[md5Hash]
			if flag && val == 0 {
				return
			}
			logger.WithFields(logrus.Fields{
				"submodule": "withdraw",
				"name":      name,
			}).Info("Transaction Waiting ", txHash, " md5Hash ", md5Hash)
		}
	}
}

func getMD5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
