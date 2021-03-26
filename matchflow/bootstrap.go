package matchflow

import (
	"github.com/open-dex/conflux-dex-audit/common"
)

var logger = common.NewLogger("matchflow")

// Start bootstraps auditor workers for MatchFlow
func Start(cfxURL string, matchURL string, assets []common.Asset, config *common.MatchflowConfig) {
	// Initialize Conflux client
	cfxClient := common.MustNewCfx(cfxURL)
	defer cfxClient.Close()

	// Initialize DEX client
	httpClient := common.NewClient(matchURL)

	assetsMap := make(map[string]*common.Contract)
	for _, asset := range assets {
		assetsMap[asset.Name] = common.GetContract(cfxClient, common.CrclABI, asset.ContractAddress)
	}
	worker := NewWorker(httpClient, cfxClient, assetsMap, config)
	worker.Start(config)

	logger.Info("matchflow auditor started")
}
