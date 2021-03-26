package boomflow

import (
	"fmt"
	"math/big"

	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/pkg/errors"
)

// EventAuditDetails represents the details of audit against CRCL event logs.
type EventAuditDetails struct {
	BalanceIncreased *common.AccountBalances
	BalanceReduced   *common.AccountBalances
}

// merge merges the changed balances from event logs into specified account balances
// and returns the updated balances of changed accounts.
func (details *EventAuditDetails) merge(accountBalances *common.AccountBalances) map[string]*big.Int {
	changed := make(map[string]*big.Int)

	for account, amount := range details.BalanceIncreased.Map() {
		accountBalances.Add(account, amount)
		changed[account] = accountBalances.Get(account)
	}

	for account, amount := range details.BalanceReduced.Map() {
		accountBalances.Add(account, new(big.Int).Neg(amount))
		changed[account] = accountBalances.Get(account)
	}

	return changed
}

// AuditEventLogs audits event logs of CRCL for the specified epoch.
func AuditEventLogs(cfx *sdk.Client, epoch *big.Int, crcls []types.Address) (map[string]EventAuditDetails, error) {
	filter := types.LogFilter{
		FromEpoch: types.NewEpochNumberBig(epoch),
		ToEpoch:   types.NewEpochNumberBig(epoch),
		Address:   crcls,
	}

	logs, err := cfx.GetLogs(filter)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to poll event logs from full node for epoch %v", epoch)
	}

	result := make(map[string]EventAuditDetails)

	for i := range logs {
		if err = updateEventAuditDetails(&logs[i], result); err != nil {
			return nil, errors.WithMessagef(err, "failed to update event audit details, log index = %v", i)
		}
	}

	return result, nil
}

func updateEventAuditDetails(log *types.Log, result map[string]EventAuditDetails) error {
	crcl := log.Address.GetHexAddress()
	details, ok := result[crcl]
	if !ok {
		details = EventAuditDetails{
			BalanceIncreased: common.NewAccountBalances(),
			BalanceReduced:   common.NewAccountBalances(),
		}
		result[crcl] = details
	}

	switch log.Topics[0] {
	case common.EventHashTransfer:
		updateOnTransfer(details, log)
	case common.EventHashDeposit:
		// do nothing due to Transfer(0, recipient, amount)
	case common.EventHashWithdraw:
		// do nothing due to Transfer(sender, 0, amount)
	case common.EventHashWrite:
		return fmt.Errorf("Write event found")
	default:
		return fmt.Errorf("unknown event hash %v", log.Topics[0])
	}

	return nil
}

func updateOnTransfer(details EventAuditDetails, log *types.Log) {
	sender, recipient, amount := decodeCrclEvent(log)

	// transfer or withdraw
	if sender != common.ZeroAddress {
		details.BalanceReduced.Add(sender, amount)

		// monitor withdraw event for market makers
		if recipient == common.ZeroAddress {
			if config.MarketMakers[sender] {
				common.Notifyf("Withdraw from market maker, address = %v, amount = %v", sender, amount)
			}
		}
	}

	// transfer or deposit
	if recipient != common.ZeroAddress {
		details.BalanceIncreased.Add(recipient, amount)
	}
}

func decodeCrclEvent(log *types.Log) (string, string, *big.Int) {
	// 0x + padding zeros(24) + address (40)
	sender := "0x" + log.Topics[1].String()[26:]
	recipient := "0x" + log.Topics[2].String()[26:]
	strData := log.Data.String()

	// ignore the 0x prefix
	amount, ok := new(big.Int).SetString(strData[2:], 16)
	if !ok {
		panic("failed to convert log data to amount, data = " + strData)
	}

	return sender, recipient, amount
}
