package cmd

import (
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/open-dex/conflux-dex-audit/matchflow"
	"github.com/spf13/cobra"
)

var (
	matchflowConfig *common.MatchflowConfig = &common.MatchflowConfig{
		FullEpoch:    "-50",
		PartialEpoch: "-50",
		DbAddress:    "tcp(127.0.0.1:3306)",
		DbPass:       "",
		InitialAudit: false,
		Pausable:     false,
		DexStartTime: "2020-01-01 00:00:00",
		DbUser:       "admin",
	}
)

var matchflowAuditCmd = &cobra.Command{
	Use:   "matchflow",
	Short: "matchflow audit subcommands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var matchflowAuditTradeCmd = &cobra.Command{
	Use:   "trade",
	Short: "Audit offchain trade and onchain balance from specific epoch",
	Run: func(cmd *cobra.Command, args []string) {
		// Get all currencies from DEX
		httpClient := common.NewClient(common.MatchflowURL)
		assets := httpClient.GetAssets()
		matchflow.Start(cfxURL, common.MatchflowURL, assets, matchflowConfig)
		select {}
	},
}

func init() {
	matchflowConfig = &common.MatchflowConfig{}
	matchflowAuditTradeCmd.Flags().StringVar(&matchflowConfig.FullEpoch, "full", "-50", "epoch to do full audit, negative value means epoch before latest_state")
	matchflowAuditTradeCmd.Flags().StringVar(&matchflowConfig.PartialEpoch, "partial", "-50", "epoch to do partial audit, negative value means epoch before latest_state")
	matchflowAuditTradeCmd.Flags().StringVar(&matchflowConfig.DbAddress, "dbaddr", "tcp(127.0.0.1:3306)", "DEX database address")
	matchflowAuditTradeCmd.Flags().StringVar(&matchflowConfig.DbPass, "dbpass", "", "DEX database password")
	matchflowAuditTradeCmd.Flags().StringVar(&matchflowConfig.DexStartTime, "dexstart", "2020-01-01 00:00:00", "DEX start time")
	matchflowAuditTradeCmd.Flags().StringVar(&matchflowConfig.DbUser, "dbuser", "admin", "DEX db user")
	matchflowAuditTradeCmd.Flags().StringVar(&common.DexAdmin, "dexadmin", "", "DEX admin address")
	matchflowAuditTradeCmd.Flags().BoolVar(&matchflowConfig.InitialAudit, "init", false, "whether check all account balance at beginning. used only when dex is paused")
	matchflowAuditTradeCmd.Flags().BoolVar(&matchflowConfig.Pausable, "matchflow-pausable", false, "whether pause matchflow when error occurs")
	matchflowAuditCmd.AddCommand(matchflowAuditTradeCmd)
	rootCmd.AddCommand(matchflowAuditCmd)
}
