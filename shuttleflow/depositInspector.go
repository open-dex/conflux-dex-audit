package shuttleflow

import (
	"fmt"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	conflux "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	//"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/syndtr/goleveldb/leveldb"
)

var walletList [][]ethCommon.Hash
var walletMap map[string]bool
var walletAddrs []string
var transferEventHash ethCommon.Hash

// DepositInspector auditor
type DepositInspector struct {
	Client    *conflux.Client
	Timeout   time.Duration
	BTCWallet string
	Factory   string
	USDT      string
	Custodian *common.Contract
	Cb        *Config
}

// Start up an auditor
func (d *DepositInspector) Start(wg *sync.WaitGroup, vp *viper.Viper) {
	transferEventHash = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	walletList = [][]ethCommon.Hash{{transferEventHash}, {}, {}, {}}
	walletMap = make(map[string]bool)
	walletAddrs = []string{}

	client, err := ethclient.Dial(d.Cb.ETHDial)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Warn("ethclient ", err.Error())
	}

	db, err := leveldb.OpenFile(d.Cb.LevelDBPath, nil)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Warn("leveldb ", err.Error())
	}

	latestBlock := GetETHLatestBlock(client)
	err = db.Put([]byte("LastBlockNumber"), []byte(latestBlock.String()), nil)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Warn("leveldb ", err.Error())
	}
	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		key := string(iter.Key())
		if key != "LastBlockNumber" {
			walletList[2] = append(walletList[2], ethCommon.HexToHash(key))
			walletMap[key] = true
			walletAddrs = append(walletAddrs, key)
		} else {
			logger.WithFields(logrus.Fields{
				"submodule": "deposit",
			}).Info("LastBlockNumber ", string(iter.Value()))
		}
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Warn("leveldb ", err.Error())
	}

	wg.Add(4)
	go d.runBTCListener(wg, vp)

	go d.runCreate2Listener(client, db, wg, vp)
	go d.runUSDTListener(client, db, wg, vp)
	go d.runETHListener(client, db, wg, vp)
}

func (d *DepositInspector) runBTCListener(wg *sync.WaitGroup, vp *viper.Viper) {
	defer wg.Done()

	prevHeight := d.Cb.BTCInitialHeight
	for {
		currentHeight := GetBTCCurrentHeight() + 1
		for prevHeight == currentHeight {
			time.Sleep(90 * time.Second)
			currentHeight = GetBTCCurrentHeight() + 1
		}

		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Debug("BTC ", prevHeight, " ", currentHeight)

		txs := GetBTCEvents(d.BTCWallet, big.NewInt(prevHeight), big.NewInt(currentHeight))

		for _, tx := range txs {
			if len(tx.Inputs) == 1 && len(tx.Outputs) == 1 && tx.Inputs[0].OutputValue >= vp.GetInt("btc.minimalDeposit") {
				d.checkCompletion("BTC", tx.Inputs[0].PrevHash+"_"+strconv.Itoa(tx.Inputs[0].OutputIndex))
			}
		}

		prevHeight = currentHeight

		time.Sleep(60 * time.Second)
	}
}

// Ethereum
func (d *DepositInspector) runCreate2Listener(client *ethclient.Client, db *leveldb.DB, wg *sync.WaitGroup, vp *viper.Viper) {
	defer wg.Done()

	file, err := os.Open(common.Create2ABI)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Warn("Create2ABI ", err.Error())
	}

	abi, err := abi.JSON(file)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Warn("abi JSON ", err.Error())
	}

	currentBlock := getETHLastBlockNumberMinusN(db, 0)
	if d.Cb.Create2Sync {
		currentBlock = big.NewInt(vp.GetInt64("eth.initialBlock"))
	}
	latestBlock := GetETHLatestBlock(client)
	for {
		wallets := GetCreate2Transactions(client, d.Factory, currentBlock, abi)

		batch := new(leveldb.Batch)
		for _, wallet := range wallets {
			flag, err := db.Has([]byte(wallet), nil)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"submodule": "deposit",
				}).Warn("leveldb ", err.Error())
			}

			if !flag {
				batch.Put([]byte(wallet), []byte(currentBlock.String()))
				walletList[2] = append(walletList[2], ethCommon.HexToHash(wallet))
				walletMap[wallet] = true
				walletAddrs = append(walletAddrs, wallet)
			}
		}

		err = db.Put([]byte("LastBlockNumber"), []byte(latestBlock.String()), nil)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"submodule": "deposit",
			}).Warn("leveldb ", err.Error())
		}

		err = db.Write(batch, nil)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"submodule": "deposit",
			}).Warn("leveldb ", err.Error())
		}

		nextBlock := GetETHLatestBlock(client)
		for latestBlock.Cmp(nextBlock) != -1 {
			nextBlock = GetETHLatestBlock(client)
			if latestBlock.Cmp(nextBlock) == -1 {
				break
			}
			time.Sleep(60 * time.Second)
		}

		currentBlock = latestBlock
		latestBlock = nextBlock

		time.Sleep(120 * time.Second)
	}
}

func (d *DepositInspector) runUSDTListener(client *ethclient.Client, db *leveldb.DB, wg *sync.WaitGroup, vp *viper.Viper) {
	defer wg.Done()

	file, err := os.Open(common.Erc20ABI)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Warn("Erc20ABI ", err.Error())
	}

	usdtAbi, err := abi.JSON(file)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Warn("abi JSON ", err.Error())
	}

	prevBlock := big.NewInt(d.Cb.ETHInitialBlock)
	delay := d.Cb.ETHDelayBlock

	for {
		currentBlock := getETHLastBlockNumberMinusN(db, delay)

		for prevBlock.Cmp(currentBlock) != -1 {
			time.Sleep(90 * time.Second)
			currentBlock = getETHLastBlockNumberMinusN(db, delay)
		}

		//fmt.Println("USDT", prevBlock.String(), currentBlock.String())
		transferLogs := GetUSDTEvents(client, d.USDT, prevBlock, currentBlock, walletList)
		for _, tLog := range transferLogs {
			event := new(LogTransfer)
			_, err := usdtAbi.Events["Transfer"].Inputs.Unpack(tLog.Data)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"submodule": "deposit",
				}).Warn("Unpack ", err.Error())
			}

			if event.Value.Cmp(big.NewInt(vp.GetInt64("usdt.minimalDeposit"))) != -1 {
				d.checkCompletion("USDT", tLog.TxHash.Hex())
			}
		}

		prevBlock = currentBlock

		time.Sleep(60 * time.Second)
	}
}

func (d *DepositInspector) runETHListener(client *ethclient.Client, db *leveldb.DB, wg *sync.WaitGroup, vp *viper.Viper) {
	defer wg.Done()

	delay := d.Cb.ETHDelayBlock
	currentBlock := big.NewInt(d.Cb.ETHInitialBlock)
	latestMinusNBlock := getETHLastBlockNumberMinusN(db, delay)

	for {
		transactions := GetETHEvents(client, currentBlock, walletMap)
		for _, tx := range transactions {
			if tx.Value().Cmp(big.NewInt(vp.GetInt64("eth.minimalDeposit"))) != -1 {
				d.checkCompletion("ETH", tx.Hash().Hex())
			}
		}

		currentBlock.Add(currentBlock, big.NewInt(1))

		for currentBlock.Cmp(latestMinusNBlock) != -1 {
			latestMinusNBlock := getETHLastBlockNumberMinusN(db, delay)
			//fmt.Println("ETH Wait", currentBlock.String(), latestBlock.String())
			if currentBlock.Cmp(latestMinusNBlock) == -1 {
				break
			}
			time.Sleep(30 * time.Second)
		}
	}
}

func (d *DepositInspector) checkCompletion(name string, txHash string) {
	flag := d.Custodian.MustGetMintedTx(txHash)
	if flag {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
			"name":      name,
		}).Info("Transaction Exists ", txHash)
	} else {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
			"name":      name,
		}).Info("Transaction Waiting ", txHash)
		go d.waitForCompletion(name, txHash)
	}
}

func (d *DepositInspector) waitForCompletion(name string, txHash string) {
	timeout := time.After(d.Timeout)
	tick := time.Tick(60 * time.Second)

	for {
		select {
		case <-timeout:
			logger.WithFields(logrus.Fields{
				"submodule": "deposit",
				"name":      name,
			}).Warn("Transaction Timeout ", txHash)

			err := fmt.Sprintf("%s deposit %s timeout: %s", name, d.Timeout.String(), txHash)
			common.Alert(module, err)
			return

		case <-tick:
			flag := d.Custodian.MustGetMintedTx(txHash)
			if flag {
				return
			}
			logger.WithFields(logrus.Fields{
				"submodule": "deposit",
			}).Info("Transaction Waiting ", txHash)
		}
	}
}

func getETHLastBlockNumberMinusN(db *leveldb.DB, delay int64) *big.Int {
	block, err := db.Get([]byte("LastBlockNumber"), nil)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Fatal("config file ", err.Error())
	}

	lastBlock, flag := new(big.Int).SetString(string(block), 10)
	if !flag {
		logger.WithFields(logrus.Fields{
			"submodule": "deposit",
		}).Fatal("failed conversion ", string(block))
	}

	return lastBlock.Sub(lastBlock, big.NewInt(delay))
}
