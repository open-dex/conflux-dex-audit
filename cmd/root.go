package cmd

import (
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"

	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/open-dex/conflux-dex-audit/boomflow"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/open-dex/conflux-dex-audit/matchflow"
	"github.com/open-dex/conflux-dex-audit/shuttleflow"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var logger = common.NewLogger("cmd")

const periodicalAuditIntervalEpochs uint64 = 5000

var (
	cfxURL         string
	shuttleflowURL string
	epoch          string
	logLevel       string
	confirmEpochs  uint64

	rootCmd = &cobra.Command{
		Use:   "conflux-dex-audit",
		Short: "Conflux DEX auditor is used to ensure the account balances in database are consistent with balances on blockchain",
		Run: func(cmd *cobra.Command, args []string) {
			start()
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfxURL, "cfx", common.CfxURLMainNet, "Conflux full node RPC URL")
	rootCmd.PersistentFlags().StringVar(&common.MatchflowURL, "matchflow", common.MatchflowURLTest, "Conflux DEX matchflow URL")
	rootCmd.PersistentFlags().StringVar(&shuttleflowURL, "shuttleflow", common.ShuttleflowURLTest, "Conflux DEX shuttleflow URL")
	rootCmd.PersistentFlags().StringVar(&epoch, "epoch", "-10", "Epoch to audit, negative value means epoch before latest_state")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level: trace, debug, info, warn and error")
	rootCmd.PersistentFlags().Uint64Var(&confirmEpochs, "confirm-epochs", 10, "Number of epochs before latest state treated as confirmed")
	rootCmd.PersistentFlags().StringVar(&common.DingDingAccessToken, "access-token", "", "Access token used to send message to alert system")
	rootCmd.PersistentFlags().StringVar(&common.DexAdminPrivKey, "admin-privkey", "", "Private key of dex admin")
	rootCmd.PersistentFlags().StringVar(&common.AesSecret, "AesSecret", "", "AesSecret")
	rootCmd.PersistentFlags().StringVar(&common.CustodianAddress, "custodian-addr", "", "custodian node address")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		setLogLevel()

		common.NumEpochsConfirmed = new(big.Int).SetUint64(confirmEpochs)
	}
}

func start() {
	// Get all currencies from DEX
	assets := common.GetAssets(common.MatchflowURL)

	epochNum := mustParseEpoch()
	logger.WithFields(logrus.Fields{
		"epoch":    epoch,
		"epochNum": epochNum,
	}).Info("start to audit Conflux DEX")

	wg := sync.WaitGroup{}

	shuttleflow.Start(cfxURL, shuttleflowURL, assets, shuttleflowConfig, &wg)
	boomflow.StartSince(cfxURL, common.MatchflowURL, epochNum, periodicalAuditIntervalEpochs, &wg)
	matchflow.Start(cfxURL, common.MatchflowURL, assets, matchflowConfig)

	wg.Wait()
}

func mustParseEpoch() *big.Int {
	epochNum, ok := new(big.Int).SetString(epoch, 10)
	if !ok {
		logger.WithError(errors.New("invalid epoch format")).WithField("epoch", epoch).Fatal("failed to parse epoch")
	}

	if epochNum.Sign() >= 0 {
		return epochNum
	}

	// latatest state - N
	cfx := common.MustNewCfx(cfxURL)
	defer cfx.Close()
	currentEpoch, err := cfx.GetEpochNumber(types.EpochLatestState)
	if err != nil {
		logger.WithError(errors.WithMessage(err, "failed to get latest state epoch number")).Fatal("failed to parse epoch")
	}

	epochNum = new(big.Int).Add(currentEpoch.ToInt(), epochNum)
	if epochNum.Sign() < 0 {
		logger.WithFields(logrus.Fields{
			"latestEpoch":    currentEpoch,
			"requestedEpoch": epochNum,
		}).Fatal("failed to parse epoch")
	}

	return epochNum
}

func setLogLevel() {
	switch strings.ToLower(logLevel) {
	case "trace":
		logrus.SetLevel(logrus.TraceLevel)
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logger.Fatalf("invalid log level %v", logLevel)
	}
}

// Execute is the command line entrypoint.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
