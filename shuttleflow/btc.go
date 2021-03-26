package shuttleflow

import (
	"github.com/blockcypher/gobcy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"math/big"
)

// TransactionRef transaction ref
type TransactionRef struct {
	TxHash        string `json:"tx_hash"`
	BlockHeight   int64  `json:"block_height"`
	TxInputN      int64  `json:"tx_input_n"`
	TxOutputN     int64  `json:"tx_output_n"`
	Value         int64  `json:"value"`
	RefBalance    int64  `json:"ref_balance"`
	Spent         bool   `json:"spent"`
	Confirmations int64  `json:"confirmations"`
	Confirmed     string `json:"confirmed"`
	DoubleSpend   bool   `json:"double_spend"`
}

// BTCBalance balance of BTC address
type BTCBalance struct {
	Address            string           `json:"address"`
	TotalReceived      int64            `json:"total_received"`
	TotalSent          int64            `json:"total_sent"`
	Balance            int64            `json:"balance"`
	UnconfirmedBalance int64            `json:"unconfirmed_balance"`
	FinalBalance       int64            `json:"final_balance"`
	NTx                int64            `json:"n_tx"`
	UnconfirmedNTx     int64            `json:"unconfirmed_n_tx"`
	FinalNTx           int64            `json:"final_n_tx"`
	TxRefs             []TransactionRef `json:"txrefs"`
}

// GetBTCBalance returns the balance of the address with error code.
func GetBTCBalance(addr string) (*big.Int, error) {
	btc := gobcy.API{"", "btc", "main"}
	result, err := btc.GetAddrBal(addr, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get BTC balance")
	}

	return big.NewInt(int64(result.Balance)), nil
}

// MustGetBTCBalance the balance of the address.
func MustGetBTCBalance(addr string) *big.Int {
	result, err := GetBTCBalance(addr)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "btc util",
		}).Warn("MustGetBTCBalance ", err.Error())
	}

	return result
}

// GetBTCEvents Get BTC events
func GetBTCEvents(addr string, fromHeight *big.Int, toHeight *big.Int) []gobcy.TX {
	btc := gobcy.API{"", "btc", "main"}
	events, err := btc.GetAddrFull(addr, map[string]string{
		"before": toHeight.String(),
		"after":  fromHeight.String(),
	})
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "btc util",
		}).Warn("GetAddrFull ", err.Error())
	}

	return events.TXs
}

// GetBTCCurrentHeight Get BTC current height
func GetBTCCurrentHeight() int64 {
	btc := gobcy.API{"", "btc", "main"}
	chainInfo, err := btc.GetChain()
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "btc util",
		}).Warn("GetChain ", err.Error())
	}

	return int64(chainInfo.Height)
}
