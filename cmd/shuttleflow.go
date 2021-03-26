package cmd

import (
	//"fmt"
	"sync"
	"time"

	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/open-dex/conflux-dex-audit/shuttleflow"
	"github.com/sirupsen/logrus"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	shuttleflowConfig *shuttleflow.Config = &shuttleflow.Config{
		TimeoutInMinute:  10,
		TimeoutInSecond:  0,
		ETHDial:          "https://mainnet.infura.io/v3/eb23d7464c694540b424bec4f2250a34",
		LevelDBPath:      "./leveldb/shuttleflow/wallet-db",
		Create2Sync:      false,
		BTCInitialHeight: 600000,
		ETHInitialBlock:  9947842,
		ETHDelayBlock:    100,
		CFXInitialBlock:  "2488514",
		BTCMinus:         5000,
	}
)

var shuttleflowAuditCmd = &cobra.Command{
	Use:   "shuttleflow",
	Short: "shuttleflow audit subcommands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var shuttleflowAuditAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Audit crosschain assets total supply",
	Run: func(cmd *cobra.Command, args []string) {
		// Get all currencies from DEX
		assets := common.GetAssets(common.MatchflowURL)

		wg := sync.WaitGroup{}

		shuttleflow.Start(cfxURL, shuttleflowURL, assets, shuttleflowConfig, &wg)

		wg.Wait()
	},
}

var shuttleflowAuditDepositCmd = &cobra.Command{
	Use:   "deposit",
	Short: "Audit crosschain deposit events",
	Run: func(cmd *cobra.Command, args []string) {
		// Read in config
		vp := viper.New()
		vp.SetConfigName("./shuttleflow/config")
		vp.AddConfigPath(".")
		err := vp.ReadInConfig()
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatal("Fatal error config file")
		}

		// Get all currencies from DEX
		httpClient := common.NewClient(common.MatchflowURL)
		assets := httpClient.GetAssets()

		cfxClient := common.MustNewCfx(cfxURL)
		defer cfxClient.Close()

		// Get all currencies from DEX
		erc777Map := make(map[string]*common.Contract)
		for _, asset := range assets {
			// Only audit crosschain assets
			if asset.CrossChain {
				erc777Map[asset.Name] = common.GetContract(cfxClient, common.Erc777ABI, asset.TokenAddress)
			}
		}

		wg := &sync.WaitGroup{}

		// Initialize deposit inspector
		depositInspector := &shuttleflow.DepositInspector{
			Client:    cfxClient,
			Timeout:   time.Duration(shuttleflowConfig.TimeoutInMinute)*time.Minute + time.Duration(shuttleflowConfig.TimeoutInSecond)*time.Second,
			BTCWallet: vp.GetString("btc.hot"),
			Factory:   vp.GetString("create2factory.prod"),
			USDT:      vp.GetString("usdt.address"),
			Custodian: common.GetContract(cfxClient, common.CustodianABI, vp.GetString("custodian.prod")),
			Cb:        shuttleflowConfig,
		}

		depositInspector.Start(wg, vp)

		// Initialize total supply auditor
		worker := &shuttleflow.Worker{
			Erc777Map:   erc777Map,
			UsdtAddress: vp.GetString("usdt.address"),
			Cb:          shuttleflowConfig,
		}
		worker.Start(wg, vp)

		wg.Wait()
	},
}

var shuttleflowAuditWithdrawCmd = &cobra.Command{
	Use:   "withdraw",
	Short: "Audit crosschain withdraw events",
	Run: func(cmd *cobra.Command, args []string) {
		// Read in config
		vp := viper.New()
		vp.SetConfigName("./shuttleflow/config")
		vp.AddConfigPath(".")
		err := vp.ReadInConfig()
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatal("Fatal error config file")
		}

		// Get all currencies from DEX
		httpClient := common.NewClient(common.MatchflowURL)
		assets := httpClient.GetAssets()

		cfxClient := common.MustNewCfx(cfxURL)
		defer cfxClient.Close()

		// Initialize Ethereum client
		ethClient, err := ethclient.Dial(shuttleflowConfig.ETHDial)

		// Get all currencies from DEX
		erc777Map := make(map[string]*common.Contract)
		for _, asset := range assets {
			// Only audit crosschain assets
			if asset.CrossChain {
				erc777Map[asset.Name] = common.GetContract(cfxClient, common.Erc777ABI, asset.TokenAddress)
			}
		}

		// Initialize withdraw inspector
		ethFactory, err := shuttleflow.BindContract(ethClient, vp.GetString("ethfactory.prod"), common.EthFactoryABI)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"submodule": "worker",
			}).Warn("BindContract ", err.Error())
		}

		withdrawInspector := &shuttleflow.WithdrawInspector{
			Client:     cfxClient,
			Timeout:    time.Duration(shuttleflowConfig.TimeoutInMinute)*time.Minute + time.Duration(shuttleflowConfig.TimeoutInSecond)*time.Second,
			Erc777Map:  erc777Map,
			BTCWallet:  vp.GetString("btc.hot"),
			ETHFactory: ethFactory,
			Cb:         shuttleflowConfig,
		}

		wg := &sync.WaitGroup{}
		wg.Add(1)
		withdrawInspector.Start(wg, vp)

		wg.Wait()
	},
}

func init() {
	shuttleflowConfig = &shuttleflow.Config{}
	shuttleflowAuditCmd.Flags().Int64Var(&shuttleflowConfig.TimeoutInMinute, "timeoutM", 10, "timeout in minutes")
	shuttleflowAuditCmd.Flags().Int64Var(&shuttleflowConfig.TimeoutInSecond, "timeoutS", 0, "timeout in seconds")
	shuttleflowAuditCmd.Flags().StringVar(&shuttleflowConfig.ETHDial, "ethurl", "https://mainnet.infura.io/v3/eb23d7464c694540b424bec4f2250a34", "ETH api url")
	shuttleflowAuditCmd.Flags().StringVar(&shuttleflowConfig.LevelDBPath, "leveldb", "./leveldb/shuttleflow/wallet-db", "path to leveldb folder")
	shuttleflowAuditCmd.Flags().BoolVar(&shuttleflowConfig.Create2Sync, "sync", false, "sync up create2 wallet addresses")
	shuttleflowAuditCmd.Flags().Int64Var(&shuttleflowConfig.BTCInitialHeight, "btcinit", 600000, "BTC initial height")
	shuttleflowAuditCmd.Flags().Int64Var(&shuttleflowConfig.ETHInitialBlock, "ethinit", 9947842, "ETH intial block number")
	shuttleflowAuditCmd.Flags().Int64Var(&shuttleflowConfig.ETHDelayBlock, "ethdelay", 100, "Number of blocks to delay for checking ETH events")

	shuttleflowAuditCmd.AddCommand(shuttleflowAuditAllCmd)

	shuttleflowAuditDepositCmd.Flags().StringVar(&shuttleflowConfig.LevelDBPath, "leveldb", "./leveldb/shuttleflow/wallet-db", "path to leveldb folder")
	shuttleflowAuditDepositCmd.Flags().Int64Var(&shuttleflowConfig.BTCInitialHeight, "btcinit", 600000, "BTC initial height")
	shuttleflowAuditDepositCmd.Flags().Int64Var(&shuttleflowConfig.ETHInitialBlock, "ethinit", 9947842, "ETH intial block number")
	shuttleflowAuditDepositCmd.Flags().BoolVar(&shuttleflowConfig.Create2Sync, "sync", false, "sync up create2 wallet addresses")
	shuttleflowAuditCmd.AddCommand(shuttleflowAuditDepositCmd)

	shuttleflowAuditWithdrawCmd.Flags().Int64Var(&shuttleflowConfig.BTCMinus, "btcminus", 5000, "BTC current height subtract provided value, which is the intial height")
	shuttleflowAuditWithdrawCmd.Flags().StringVar(&shuttleflowConfig.CFXInitialBlock, "cfxinit", "2488514", "CFX intial block number")
	shuttleflowAuditCmd.AddCommand(shuttleflowAuditWithdrawCmd)

	rootCmd.AddCommand(shuttleflowAuditCmd)
}
