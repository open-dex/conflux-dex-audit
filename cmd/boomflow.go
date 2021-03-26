package cmd

import (
	"fmt"
	"sync"

	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/Conflux-Chain/go-conflux-sdk/types/cfxaddress"
	"github.com/open-dex/conflux-dex-audit/boomflow"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	asset                      string
	showDetails                bool
	balanceAuditIntervalEpochs uint64
)

var boomflowAuditCmd = &cobra.Command{
	Use:   "boomflow",
	Short: "Start Boomflow audit service",
	Run: func(cmd *cobra.Command, args []string) {
		epochNum := mustParseEpoch()
		logger.WithFields(logrus.Fields{
			"epoch":    epochNum,
			"interval": balanceAuditIntervalEpochs,
		}).Info("start to audit boomflow")

		wg := sync.WaitGroup{}
		boomflow.StartSince(cfxURL, common.MatchflowURL, epochNum, balanceAuditIntervalEpochs, &wg)
		wg.Wait()
	},
}

var boomflowAuditBalanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "Audit balance for specific epoch",
	Run: func(cmd *cobra.Command, args []string) {
		am := boomflow.NewAuditManager(cfxURL, common.MatchflowURL)
		defer am.Close()

		epochNum := mustParseEpoch()

		logger.WithFields(logrus.Fields{
			"asset": asset,
			"epoch": epochNum,
		}).Info("begin to audit blanace")

		details, err := am.AuditBalanceForAsset(asset, epochNum)

		if err != nil {
			logger.WithError(err).Error("failed to audit balance")
		} else if showDetails {
			logger.WithField("details", *details).Info("succeed to audit balance")
		} else {
			logger.Info("succeed to audit balance")
		}
	},
}

var boomflowAuditEventLogsCmd = &cobra.Command{
	Use:   "event",
	Short: "Audit event logs for specific epoch",
	Run: func(cmd *cobra.Command, args []string) {
		cfxClient := common.MustNewCfx(cfxURL)
		defer cfxClient.Close()

		epochNum := mustParseEpoch()

		client := common.NewClient(common.MatchflowURL)
		assets := client.GetAssets()
		var crcls []types.Address
		for _, asset := range assets {
			crcls = append(crcls, cfxaddress.MustNewFromHex(asset.ContractAddress, common.GetNetworkId()))
		}

		logger.WithFields(logrus.Fields{
			"epoch":  epochNum,
			"assets": len(assets),
		}).Info("start to audit event logs")

		allDetails, err := boomflow.AuditEventLogs(cfxClient, epochNum, crcls)

		if err != nil {
			logger.WithError(err).Error("failed to audit event logs")
		} else if showDetails {
			logger.WithField("details", fmt.Sprintf("%+v", allDetails)).Info("succeed to audit event logs")
		} else {
			logger.Info("succeed to audit event logs")
		}
	},
}

func init() {
	boomflowAuditCmd.Flags().Uint64Var(&balanceAuditIntervalEpochs, "interval", 5000, "Number of epochs to audit balance once")

	boomflowAuditBalanceCmd.Flags().StringVar(&asset, "asset", "", "Asset to audit")
	boomflowAuditBalanceCmd.MarkFlagRequired("asset")
	boomflowAuditBalanceCmd.Flags().BoolVar(&showDetails, "details", false, "Whether to show account balance in details")
	boomflowAuditCmd.AddCommand(boomflowAuditBalanceCmd)

	boomflowAuditEventLogsCmd.Flags().BoolVar(&showDetails, "details", false, "Whether to show balance changed accounts in details")
	boomflowAuditCmd.AddCommand(boomflowAuditEventLogsCmd)

	rootCmd.AddCommand(boomflowAuditCmd)
}
