package boomflow

import (
	"fmt"
	"math/big"

	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BalanceAuditDetails represents the details of audit against balance.
type BalanceAuditDetails struct {
	TotalSupply     *big.Int                // total supply in CRCL
	AccountBalances *common.AccountBalances // account balances in CRCL
}

// AssetAuditor audits contract data for a single asset.
type AssetAuditor struct {
	asset             common.Asset
	erc777            *common.Contract
	crcl              *common.Contract
	logger            logrus.FieldLogger
	balanceByTransfer *big.Int // balance that transfered to ERC777, not by send method
}

// NewAssetAuditor creates an instance of NewAssetAuditor.
func NewAssetAuditor(asset common.Asset, cfx *sdk.Client) *AssetAuditor {
	transfer := config.getBalanceByTransfer(asset.Name)
	logger.Debug("diff for", asset.Name, "is ", transfer)
	return &AssetAuditor{
		asset:  asset,
		erc777: common.GetContract(cfx, common.Erc777ABI, asset.TokenAddress),
		crcl:   common.GetContract(cfx, common.CrclABI, asset.ContractAddress),
		logger: logger.WithFields(logrus.Fields{
			"asset": asset.Name,
		}),
		balanceByTransfer: transfer,
	}
}

// AuditTotalSupply audits the total supply in CRCL against the balance of CRCL account in ERC777.
func (auditor *AssetAuditor) AuditTotalSupply(epochNum *big.Int) (*big.Int, error) {
	epoch := types.NewEpochNumberBig(epochNum)

	total, err := auditor.crcl.TotalSupply(epoch)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get total supply in CRCL")
	}

	balance, err := auditor.erc777.BalanceOf(auditor.crcl.Address(), epoch)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get balance of CRCL account in ERC777")
	}

	balanceBySend := new(big.Int).Sub(balance, auditor.balanceByTransfer)

	if total.Cmp(balanceBySend) != 0 {
		return nil, fmt.Errorf("inconsistent balance, CRCL.totalSupply = %v, "+
			"ERC777.balanceBySend = %v, Diff = %v, config diff = %v",
			total, balanceBySend, total.Sub(total, balanceBySend), auditor.balanceByTransfer)
	}

	return total, nil
}

// AuditAccountBalance queries account list and calculates the sum of their balances in CRCL.
// Note, this kind of audit depends on the data migration API in CRCL, which may be removed in future.
func (auditor *AssetAuditor) AuditAccountBalance(epochNum *big.Int) (*BalanceAuditDetails, error) {
	logger := auditor.logger.WithField("epoch", epochNum)

	epoch := types.NewEpochNumberBig(epochNum)

	total, err := auditor.crcl.TotalAccount(epoch)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get number of accounts in CRCL")
	}

	logger.WithField("total", total).Debug("succeed to query total accounts")

	result := BalanceAuditDetails{
		TotalSupply:     common.Big0,
		AccountBalances: common.NewAccountBalances(),
	}

	for offset := big.NewInt(0); offset.Cmp(total) < 0; {
		accounts, err := auditor.crcl.ListAccounts(offset, epoch)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to list accounts since offset %v", offset)
		}

		logger.WithFields(logrus.Fields{
			"offset": offset,
			"limit":  len(accounts),
		}).Trace("begin to query balance of accounts")

		for _, account := range accounts {
			balance, err := auditor.crcl.BalanceOf(account, epoch)
			if err != nil {
				return nil, errors.WithMessagef(err, "failed to get balance of account %v", account)
			}

			result.TotalSupply = new(big.Int).Add(result.TotalSupply, balance)
			result.AccountBalances.Add(account, balance)
		}

		offset = new(big.Int).Add(offset, big.NewInt(int64(len(accounts))))
	}

	return &result, nil
}

// AuditBalance audits balance against ERC777 and CRCL contracts. Generally, this kind of audit
// is launched periodically, e.g. every one hour. The audit items includes:
// 1) ERC777.balanceOf(CRCL)
// 2) CRCL.totalSupply()
// 3) Sum(CRCL.balanceOf(account))
func (auditor *AssetAuditor) AuditBalance(epoch *big.Int) (*BalanceAuditDetails, error) {
	logger := auditor.logger.WithField("epoch", epoch)

	total, err := auditor.AuditTotalSupply(epoch)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to audit total supply")
	}
	logger.WithField("total", total).Debug("succeed to audit total supply in CRCL")

	details, err := auditor.AuditAccountBalance(epoch)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to audit account balances")
	}
	logger.WithField("sum", details.TotalSupply).Debug("succeed to audit account balances in CRCL")

	if details.TotalSupply.Cmp(total) != 0 {
		return nil, fmt.Errorf("inconsistent balance, CRCL.totalSupply = %v, Sum(CRCL.balanceOf(account)) = %v, Diff = %v", total, details.TotalSupply, total.Sub(total, details.TotalSupply))
	}

	return details, nil
}
