package boomflow

import (
	"fmt"
	"math/big"

	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/Conflux-Chain/go-conflux-sdk/types/cfxaddress"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// AuditManager audits data for all ERC777 and CRCL contracts of boomflow.
type AuditManager struct {
	Cfx *sdk.Client

	auditors         map[string]*AssetAuditor // key is asset name
	lastAuditEpoch   *big.Int
	lastAuditDetails map[string]*BalanceAuditDetails // key is asset name

	pollLogAddresses []types.Address
	crcl2AssetMap    map[string]string // crcl address to asset name map
}

// NewAuditManager creates an instance of AuditManager.
func NewAuditManager(cfxURL, matchflowURL string) *AuditManager {
	assets := common.GetAssets(matchflowURL)
	return NewAuditManagerWithAssets(cfxURL, assets)
}

// NewAuditManagerWithAssets creates an instance of AuditManager.
func NewAuditManagerWithAssets(cfxURL string, assets []common.Asset) *AuditManager {
	cfx := common.MustNewCfx(cfxURL)

	am := AuditManager{
		Cfx:              cfx,
		auditors:         make(map[string]*AssetAuditor),
		lastAuditDetails: make(map[string]*BalanceAuditDetails),
		crcl2AssetMap:    make(map[string]string),
	}

	for _, asset := range assets {
		am.auditors[asset.Name] = NewAssetAuditor(asset, cfx)
		am.pollLogAddresses = append(am.pollLogAddresses, cfxaddress.MustNewFromHex(asset.ContractAddress, common.GetNetworkId()))
		am.crcl2AssetMap[asset.ContractAddress] = asset.Name
	}

	return &am
}

// Close releases resources hold by Boomflow auditor.
func (am *AuditManager) Close() {
	am.Cfx.Close()
}

// AuditBalance audits balance for the specified epoch.
func (am *AuditManager) AuditBalance(epoch *big.Int) error {
	logger := logger.WithField("epoch", epoch)

	// Could use goroutine for each asset if perf is low.
	// Note, do not violate the throttling on full node.
	for asset, auditor := range am.auditors {
		logger.WithField("asset", asset).Debug("begin to audit balance")
		details, err := auditor.AuditBalance(epoch)
		if err != nil {
			return errors.WithMessagef(err, "failed to audit balance for asset %v", asset)
		}

		am.lastAuditDetails[asset] = details
	}

	am.lastAuditEpoch = epoch

	return nil
}

// AuditBalanceForAsset audits balance for specified asset.
func (am *AuditManager) AuditBalanceForAsset(asset string, epoch *big.Int) (*BalanceAuditDetails, error) {
	auditor, ok := am.auditors[asset]
	if !ok {
		return nil, fmt.Errorf("invalid asset %v", asset)
	}

	details, err := auditor.AuditBalance(epoch)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to audit balance for asset %v", asset)
	}

	return details, nil
}

// AuditNextEpoch audits event logs in CRCL for the next epoch of last balance audit details.
func (am *AuditManager) AuditNextEpoch() error {
	if len(am.auditors) != len(am.lastAuditDetails) {
		return fmt.Errorf("any asset has no baseline yet")
	}

	epoch := new(big.Int).Add(am.lastAuditEpoch, common.Big1)
	logger.WithFields(logrus.Fields{
		"epochBasedOn": am.lastAuditEpoch,
		"epochToAudit": epoch,
	}).Trace("begin to audit event logs")

	// audit total supply against ERC777
	for asset, auditor := range am.auditors {
		total, err := auditor.AuditTotalSupply(epoch)
		if err != nil {
			return errors.WithMessagef(err, "failed to audit total supply in CRCL, asset = %v, epoch = %v", asset, epoch)
		}

		am.lastAuditDetails[asset].TotalSupply = total
	}

	// audit event logs
	allDetails, err := AuditEventLogs(am.Cfx, epoch, am.pollLogAddresses)
	if err != nil {
		return errors.WithMessagef(err, "failed to audit event logs for epoch %v", epoch)
	}

	for crcl, details := range allDetails {
		if err = am.auditBalanceChange(crcl, details, epoch); err != nil {
			return errors.WithMessagef(err, "failed to audit changed balances, epoch = %v, asset = %v (%v)", epoch, am.crcl2AssetMap[crcl], crcl)
		}
	}

	am.lastAuditEpoch = epoch

	return nil
}

func (am *AuditManager) auditBalanceChange(crcl string, details EventAuditDetails, epochNum *big.Int) error {
	asset, ok := am.crcl2AssetMap[crcl]
	if !ok {
		return fmt.Errorf("cannot find asset for CRCL address, epoch = %v, CRCL = %v", epochNum, crcl)
	}

	logger.WithFields(logrus.Fields{
		"asset":     asset,
		"epoch":     epochNum,
		"increased": details.BalanceIncreased,
		"reduced":   details.BalanceReduced,
	}).Debug("balance changed")

	baseline := am.lastAuditDetails[asset]
	changed := details.merge(baseline.AccountBalances)

	logger.WithField("changed", changed).Debug("new balances for changed accounts")

	// check the local updated balances with total supply
	sum := baseline.AccountBalances.Sum()
	if baseline.TotalSupply.Cmp(sum) != 0 {
		return fmt.Errorf("inconsistent balance, total supply = %v, sum(accounts) = %v", baseline.TotalSupply, sum)
	}

	epoch := types.NewEpochNumberBig(epochNum)

	// check changed balance on chain
	for account, balance := range changed {
		if balance.Sign() < 0 {
			return fmt.Errorf("the changed account balance is negative, asset = %v, epoch = %v, account = %v, balance = %v", asset, epochNum, account, balance)
		}

		actualBalance, err := am.auditors[asset].crcl.BalanceOf(account, epoch)
		if err != nil {
			return errors.WithMessagef(err, "failed to get balance on chain, asset = %v, epoch = %v, account = %v", asset, epochNum, account)
		}

		if balance.Cmp(actualBalance) != 0 {
			return fmt.Errorf("inconsistent balance, audit = %v, actual = %v, asset = %v, epoch = %v, account = %v", balance, actualBalance, asset, epochNum, account)
		}
	}

	return nil
}
