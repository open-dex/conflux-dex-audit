package shuttleflow

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Config configuration for shuttleflow auditor
type Config struct {
	TimeoutInMinute  int64
	TimeoutInSecond  int64
	ETHDial          string
	LevelDBPath      string
	Create2Sync      bool
	BTCInitialHeight int64
	ETHInitialBlock  int64
	ETHDelayBlock    int64
	CFXInitialBlock  string
	BTCMinus         int64
}

// logger is the global logger of Shuttleflow module.
const module = "shuttleflow"

var logger = common.NewLogger(module)

// Start bootstraps auditor workers for ShuttleFlow
func Start(cfxURL string, shuttleURL string, assets []common.Asset, cb *Config, wg *sync.WaitGroup) {
	// Read in config
	vp := viper.New()
	vp.SetConfigName("./shuttleflow/config")
	vp.AddConfigPath(".")
	err := vp.ReadInConfig()
	if err != nil {
		logger.WithError(errors.WithMessage(err, "ReadInConfig")).Fatal("failed to parse config file")
	}

	// Initialize Conflux client
	cfxClient := common.MustNewCfx(cfxURL)
	defer cfxClient.Close()

	// Initialize Ethereum client
	ethClient, err := ethclient.Dial(cb.ETHDial)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "bootstrap",
		}).Panic("Dial ", err.Error())
	}

	// Get all currencies from DEX
	erc777Map := make(map[string]*common.Contract)
	for _, asset := range assets {
		// Only audit crosschain assets
		if asset.CrossChain {
			erc777Map[asset.Name] = common.GetContract(cfxClient, common.Erc777ABI, asset.TokenAddress)
		}
	}

	// Initialize deposit inspector
	depositInspector := &DepositInspector{
		Client:    cfxClient,
		Timeout:   time.Duration(cb.TimeoutInMinute)*time.Minute + time.Duration(cb.TimeoutInSecond)*time.Second,
		BTCWallet: vp.GetString("btc.hot"),
		Factory:   vp.GetString("create2factory.prod"),
		USDT:      vp.GetString("usdt.address"),
		Custodian: common.GetContract(cfxClient, common.CustodianABI, vp.GetString("custodian.prod")),
		Cb:        cb,
	}

	depositInspector.Start(wg, vp)

	// Initialize withdraw inspector
	ethFactory, err := BindContract(ethClient, vp.GetString("ethfactory.prod"), common.EthFactoryABI)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "bootstrap",
		}).Panic("BindContract ", err.Error())
	}

	withdrawInspector := &WithdrawInspector{
		Client:     cfxClient,
		Timeout:    time.Duration(cb.TimeoutInMinute)*time.Minute + time.Duration(cb.TimeoutInSecond)*time.Second,
		Erc777Map:  erc777Map,
		BTCWallet:  vp.GetString("btc.hot"),
		ETHFactory: ethFactory,
		Cb:         cb,
	}

	withdrawInspector.Start(wg, vp)

	// Initialize total supply auditor
	worker := &Worker{
		Erc777Map:   erc777Map,
		UsdtAddress: vp.GetString("usdt.address"),
		Cb:          cb,
	}

	worker.Start(wg, vp)

	// All started
	logger.WithFields(logrus.Fields{
		"timeout": depositInspector.Timeout,
	}).Info("start to audit shuttleflow")
}
