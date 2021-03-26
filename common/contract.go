package common

import (
	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/Conflux-Chain/go-conflux-sdk/types/cfxaddress"
	"github.com/ethereum/go-ethereum/common"
	"io/ioutil"
	"math/big"
)

const (
	// FcABI FC ABI
	FcABI = "./common/build/FC.abi"
	// CrclABI CRC-L ABI
	CrclABI = "./common/build/CRCL.abi"
	// Erc777ABI ERC777 ABI
	Erc777ABI = "./common/build/ERC777.abi"
	// BoomflowABI Boomflow ABI
	BoomflowABI = "./common/build/Boomflow.abi"
	// CustodianABI Custodian Core ABI
	CustodianABI = "./common/build/CustodianCore.abi"
	// Create2ABI Create2Factory ABI
	Create2ABI = "./common/build/Create2Factory.abi"
	// Erc20ABI ERC20 ABI
	Erc20ABI = "./common/build/ERC20.abi"
	// EthFactoryABI EthFactory ABI
	EthFactoryABI = "./common/build/EthFactory.abi"
) // const for abi paths

// Contract represents an smart contract of Conflux DEX.
type Contract struct {
	Contract *sdk.Contract
}

// GetContract creates an instance of contract.
func GetContract(client *sdk.Client, abiPath string, address string) *Contract {
	abi, err := ioutil.ReadFile(abiPath)
	if err != nil {
		panic(err)
	}

	deployedAt := cfxaddress.MustNewFromHex(address, GetNetworkId())
	contract, err := client.GetContract(abi, &deployedAt)
	if err != nil {
		panic(err)
	}
	return &Contract{
		Contract: contract,
	}
}

// Address returns the contract address.
func (c *Contract) Address() string {
	return c.Contract.Address.String()
}

func (c *Contract) buildOption(epoch ...*types.Epoch) *types.ContractMethodCallOption {
	var option types.ContractMethodCallOption

	if len(epoch) == 1 {
		option.Epoch = epoch[0]
	}

	return &option
}

// TotalSupply returns the total supply of this CRCL contract.
func (c *Contract) TotalSupply(epoch ...*types.Epoch) (*big.Int, error) {
	option := c.buildOption(epoch...)
	totalSupply := new(big.Int)

	if err := c.Contract.Call(option, &totalSupply, "totalSupply"); err != nil {
		return nil, err
	}

	return totalSupply, nil
}

// MustGetTotalSupply returns the specified total supply of this contract.
func (c *Contract) MustGetTotalSupply(epoch ...*types.Epoch) *big.Int {
	result, err := c.TotalSupply(epoch...)
	if err != nil {
		panic(err)
	}

	return result
}

// BalanceOf returns the balance of specified account.
func (c *Contract) BalanceOf(account string, epoch ...*types.Epoch) (*big.Int, error) {
	option := c.buildOption(epoch...)
	balance := new(big.Int)
	addr, err := cfxaddress.New(account, GetNetworkId())
	if err != nil {
		panic(err)
	}

	if err := c.Contract.Call(option, &balance, "balanceOf", addr.MustGetCommonAddress()); err != nil {
		return nil, err
	}
	return balance, nil
}

// MustGetBalanceOf returns the balance of specified account.
func (c *Contract) MustGetBalanceOf(account string, epoch ...*types.Epoch) *big.Int {
	result, err := c.BalanceOf(account, epoch...)
	if err != nil {
		panic(err)
	}

	return result
}

// TotalAccount returns total number of accounts in CRCL.
func (c *Contract) TotalAccount(epoch ...*types.Epoch) (*big.Int, error) {
	option := c.buildOption(epoch...)
	total := new(big.Int)

	if err := c.Contract.Call(option, &total, "accountTotal"); err != nil {
		return nil, err
	}

	return total, nil
}

// ListAccounts lists account since specified offset in CRCL.
func (c *Contract) ListAccounts(offset *big.Int, epoch ...*types.Epoch) ([]string, error) {
	option := c.buildOption(epoch...)
	var accounts [100]common.Address

	if err := c.Contract.Call(option, &accounts, "accountList", offset); err != nil {
		return nil, err
	}

	var result []string
	for _, account := range accounts {
		addr := account.Hex()
		if addr == ZeroAddress {
			break
		}
		result = append(result, addr)
	}

	return result, nil
}

// ListAllAccounts lists all accounts in CRCL.
func (c *Contract) ListAllAccounts(epoch *types.Epoch) []string {
	total, err := c.TotalAccount(epoch)
	if err != nil {
		panic(err)
	}
	addresses := []string{}
	for offset := big.NewInt(0); offset.Cmp(total) < 0; {
		ret, err := c.ListAccounts(offset, epoch)
		if err != nil {
			panic(err)
		}
		addresses = append(addresses, ret...)
		offset.Add(offset, big.NewInt(int64(len(ret))))
	}
	return addresses
}

// MintedTx checks if the transaction has been executed with error.
func (c *Contract) MintedTx(txHash string, epoch ...*types.Epoch) (bool, error) {
	option := c.buildOption(epoch...)
	flag := new(bool)

	if err := c.Contract.Call(option, &flag, "minted_tx", txHash); err != nil {
		return false, err
	}

	return *flag, nil
}

// MustGetMintedTx checks if the transaction has been executed.
func (c *Contract) MustGetMintedTx(txHash string, epoch ...*types.Epoch) bool {
	result, err := c.MintedTx(txHash, epoch...)
	if err != nil {
		panic(err)
	}

	return result
}

// MintedTxList returns the list of all minted transactions.
func (c *Contract) MintedTxList(index int64, epoch ...*types.Epoch) string {
	option := c.buildOption(epoch...)
	var list string

	if err := c.Contract.Call(option, &list, "minted_tx_list", big.NewInt(index)); err != nil {
		panic(err)
	}

	return list
}
