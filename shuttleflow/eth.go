package shuttleflow

import (
	//"fmt"
	//"strings"
	"context"
	"math/big"
	"os"
	//"encoding/hex"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/blockcypher/gobcy"
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/sirupsen/logrus"
)

// LogTransfer ..
type LogTransfer struct {
	From  common.Address
	To    common.Address
	Value *big.Int
}

// GetETHBalance returns the balance of the address with error code.
func GetETHBalance(client *ethclient.Client, addr string) (*big.Int, error) {
	account := common.HexToAddress(addr)
	result, err := client.BalanceAt(context.Background(), account, nil)
	if err != nil {
		return nil, err
	}

	balance := new(big.Int)
	balance.SetString(result.String(), 10)

	return balance, nil
}

// GetUSDTBalance returns the balance of the address with error code.
func GetUSDTBalance(client *ethclient.Client, addr string, usdt *bind.BoundContract, opts *bind.CallOpts) (*big.Int, error) {
	account := common.HexToAddress(addr)

	result := []interface{}{new(big.Int)}
	//result := &struct{ Balance *big.Int }{}
	//var result []interface{}
	if err := usdt.Call(opts, &result, "balanceOf", account); err != nil {
		return nil, err
	}

	balance := new(big.Int)
	balance.SetString((result[0].(*big.Int)).String(), 10)

	return balance, nil
}

// MustGetETHBalance returns the balance of the address.
func MustGetETHBalance(client *ethclient.Client, addr string) *big.Int {
	result, err := GetETHBalance(client, addr)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "eth util",
		}).Warn("MustGetETHBalance ", err.Error())
	}

	return result
}

// MustGetUSDTBalance returns the balance of the address.
func MustGetUSDTBalance(client *ethclient.Client, addr string, usdt *bind.BoundContract, opts *bind.CallOpts) *big.Int {
	result, err := GetUSDTBalance(client, addr, usdt, opts)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "eth util",
		}).Warn("MustGetUSDTBalance ", err.Error())
	}

	return result
}

// GetETHEventsByAPI Get ETH events by Blockcypher
func GetETHEventsByAPI(addr string, fromHeight *big.Int, toHeight *big.Int) []gobcy.TXRef {
	eth := gobcy.API{"", "eth", "main"}
	events, err := eth.GetAddr(addr, map[string]string{
		"before": toHeight.String(),
		"after":  fromHeight.String(),
	})
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "eth util",
		}).Warn("GetAddr ", err.Error())
	}

	return events.TXRefs
}

// GetCreate2Transactions Get create2-generated addresses from Blockcypher
func GetCreate2Transactions(client *ethclient.Client, addr string, blockNumber *big.Int, abi ethabi.ABI) []string {
	transactions := GetETHEventsByAPI(addr[2:], blockNumber, nil)

	var wallets []string
	for _, transaction := range transactions {
		tx, _, err := client.TransactionByHash(context.Background(), common.HexToHash(transaction.TXHash))
		if err != nil {
			logger.WithFields(logrus.Fields{
				"submodule": "eth util",
			}).Debug("TransactionByHash ", transaction.TXHash, " ignore")
			continue
		}

		if tx.To() != nil && tx.To().Hex() == addr {
			// Ensure that the transaction execution succeed
			receipt, err := client.TransactionReceipt(context.Background(), tx.Hash())
			if err != nil {
				logger.WithFields(logrus.Fields{
					"submodule": "eth util",
				}).Warn("TransactionReceipt ", err.Error())
			}

			if receipt.Status == 1 {
				// Decode `deploy` inputs
				params := &struct {
					Code []byte
					Salt *big.Int
				}{nil, new(big.Int)}

				if _, err := abi.Methods["deploy"].Inputs.Unpack(tx.Data()[4:]); err != nil {
					logger.WithFields(logrus.Fields{
						"submodule": "eth util",
					}).Warn("Unpack ", err.Error())
				}

				origin := common.HexToAddress(addr)
				salt := common.BigToHash(params.Salt)
				codeAndHash := &codeAndHash{code: params.Code}

				address := crypto.CreateAddress2(origin, salt, codeAndHash.Hash().Bytes())
				logger.WithFields(logrus.Fields{
					"submodule": "eth util",
				}).Info("Found ETH wallet contract ", address.Hex())

				wallets = append(wallets, address.Hex())
			}
		}
	}

	return wallets
}

// GetUSDTEvents Get USDT events
func GetUSDTEvents(client *ethclient.Client, addr string, fromBlock *big.Int, toBlock *big.Int, topics [][]common.Hash) []types.Log {
	usdtAddress := common.HexToAddress(addr)
	query := ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: []common.Address{usdtAddress},
		Topics:    topics,
	}

	logs, err := client.FilterLogs(context.Background(), query)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "eth util",
		}).Warn("FilterLogs ", err.Error())
	}

	return logs
}

// GetETHEvents Get ETH events
func GetETHEvents(client *ethclient.Client, blockNumber *big.Int, walletMap map[string]bool) []*types.Transaction {
	var transactions []*types.Transaction

	block, err := client.BlockByNumber(context.Background(), blockNumber)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "eth util",
		}).Warn("BlockByNumber ", err.Error())
	}

	for _, tx := range block.Transactions() {
		if tx.To() != nil && len(tx.Data()) == 0 && walletMap[tx.To().Hex()] {
			// Ensure that the transaction execution succeed
			receipt, err := client.TransactionReceipt(context.Background(), tx.Hash())
			if err != nil {
				logger.WithFields(logrus.Fields{
					"submodule": "eth util",
				}).Warn("TransactionReceipt ", err.Error())
			}

			if receipt.Status == 1 {
				transactions = append(transactions, tx)
			}
		}
	}

	return transactions
}

// MustGetBurnedTx ..
func MustGetBurnedTx(ethFactory *bind.BoundContract, txHash string, opts *bind.CallOpts) bool {
	result := []interface{}{false}
	if err := ethFactory.Call(opts, &result, "burned_tx", txHash); err != nil {
		panic(err)
	}

	return result[0].(bool)
}

// GetETHLatestBlock Get the latest ETH block
func GetETHLatestBlock(client *ethclient.Client) *big.Int {
	block, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "eth util",
		}).Warn("BlockByNumber ", err.Error())
	}

	return block.Number()
}

type codeAndHash struct {
	code []byte
	hash common.Hash
}

// Hash ..
func (c *codeAndHash) Hash() common.Hash {
	if c.hash == (common.Hash{}) {
		c.hash = crypto.Keccak256Hash(c.code)
	}
	return c.hash
}

// BindContract binds a generic wrapper to an already deployed contract.
func BindContract(client *ethclient.Client, addr string, abiPath string) (*bind.BoundContract, error) {
	file, err := os.Open(abiPath)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "eth util",
		}).Warn("Open ", abiPath, " ", err.Error())
	}

	parsed, err := ethabi.JSON(file)
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(common.HexToAddress(addr), parsed, client, nil, nil), nil
}
