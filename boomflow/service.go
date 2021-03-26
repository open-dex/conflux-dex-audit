package boomflow

import (
	"math/big"
	"sync"
	"time"

	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// logger is the global logger of Boomflow module.
const module = "boomflow"

var logger = common.NewLogger(module)

// StartSince starts a goroutine to audit Boomflow since specified epoch continously.
// Firstly, audit the account balances at specific epoch as a baseline.
// Then, audit the event logs epoch by epoch.
//
// Moreover, the service will audit the account balances periodically, base on which
// to audit event logs epoch by epoch.
func StartSince(cfxURL, matchflowURL string, epochSince *big.Int, numEpochsToAuditBalances uint64, wg *sync.WaitGroup) {
	if numEpochsToAuditBalances == 0 {
		logger.Fatal("numEpochsToAuditBalances is zero")
	}

	wg.Add(1)
	go audit(cfxURL, matchflowURL, epochSince, numEpochsToAuditBalances, wg)
}

func audit(cfxURL, matchflowURL string, epoch *big.Int, numEpochsToAuditBalances uint64, wg *sync.WaitGroup) {
	defer wg.Done()

	am := NewAuditManager(cfxURL, matchflowURL)
	defer am.Close()

	logger.WithFields(logrus.Fields{
		"epoch":                    epoch,
		"numEpochsToAuditBalances": numEpochsToAuditBalances,
	}).Debug("start to audit Boomflow")

	delta := new(big.Int).SetUint64(numEpochsToAuditBalances - 1)
	epochFrom := epoch

	for {
		epochTo := new(big.Int).Add(epochFrom, delta)

		logger.WithFields(logrus.Fields{
			"epochFrom": epochFrom,
			"epochTo":   epochTo,
		}).Info("new round begin")

		if err := auditRound(am, epochFrom, epochTo); err != nil {
			logger.WithError(err).Error("failed to audit Boomflow")
			common.Alert(module, err.Error())
			break
		}

		epochFrom = new(big.Int).Add(epochTo, common.Big1)
	}

	// TODO notify other auditors;
	// common.AlertMatchflow()
}

func auditRound(am *AuditManager, epochFrom, epochTo *big.Int) error {
	// audit balance as baseline
	logger.WithField("epoch", epochFrom).Debug("begin to audit balances")
	waitForEpochConfirmed(am.Cfx, epochFrom)
	if err := am.AuditBalance(epochFrom); err != nil {
		return errors.WithMessagef(err, "failed to audit balances for epoch %v", epochFrom)
	}

	logger.WithField("epoch", epochFrom).Info("succeed to audit balance and continue to audit event logs")

	// audit event logs epoch by epoch
	for am.lastAuditEpoch.Cmp(epochTo) < 0 {
		logger.WithField("epochBasedOn", am.lastAuditEpoch).Trace("begin to audit event logs")
		waitForEpochConfirmed(am.Cfx, am.lastAuditEpoch)
		if err := am.AuditNextEpoch(); err != nil {
			return errors.WithMessagef(err, "failed to audit event logs, epochBasedOn = %v", am.lastAuditEpoch)
		}
	}

	return nil
}

func waitForEpochConfirmed(cfx *sdk.Client, epoch *big.Int) {
	for {
		current, err := cfx.GetEpochNumber(types.EpochLatestState)
		if err != nil {
			logger.WithError(err).Warn("failed to get epoch number from full node")
			time.Sleep(time.Second)
			continue
		}

		confirmedEpoch := new(big.Int).Sub(current.ToInt(), common.NumEpochsConfirmed)

		if epoch.Cmp(confirmedEpoch) > 0 {
			logger.WithFields(logrus.Fields{
				"epochAudit":       epoch,
				"epochConfirmed":   confirmedEpoch,
				"epochLatestState": current,
			}).Trace("audit epoch is not confirmed yet")
			time.Sleep(time.Second)
			continue
		}

		break
	}
}
