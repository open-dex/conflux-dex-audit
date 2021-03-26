package shuttleflow

import (
	//"fmt"
	"math/big"

	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/pkg/errors"
)

// GetCFXEvents ..
func GetCFXEvents(cfx *sdk.Client, epoch *big.Int, erc777s []types.Address, topics [][]types.Hash) ([]types.Log, error) {
	filter := types.LogFilter{
		FromEpoch: types.NewEpochNumberBig(epoch),
		ToEpoch:   types.NewEpochNumberBig(epoch),
		Address:   erc777s,
		Topics:    topics,
	}

	logs, err := cfx.GetLogs(filter)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to poll event logs from full node for epoch %v", epoch)
	}

	return logs, nil
}
