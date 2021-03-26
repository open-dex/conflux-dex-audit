package boomflow

import (
	"encoding/json"
	"io/ioutil"
	"math/big"

	"github.com/open-dex/conflux-dex-audit/common"
)

var config = boomflowConfig{
	MarketMakers:       make(map[string]bool),
	BalancesByTransfer: make(map[string]string),
}

type boomflowConfig struct {
	MarketMakers       map[string]bool   `json:"marketMakers"`
	BalancesByTransfer map[string]string `json:"balancesByTransfer"`
}

func (config boomflowConfig) getBalanceByTransfer(asset string) *big.Int {
	balance, ok := config.BalancesByTransfer[asset]
	if !ok {
		return common.Big0
	}

	result, ok := new(big.Int).SetString(balance, 10)
	if !ok {
		return common.Big0
	}

	return result
}

func init() {
	data, err := ioutil.ReadFile("./boomflow/config.json")
	if err != nil {
		logger.WithError(err).Fatal("failed to read config file")
	}

	if err = json.Unmarshal(data, &config); err != nil {
		logger.WithError(err).Fatal("failed to unmarshal config")
	}
}
