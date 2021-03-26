package shuttleflow

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	//conflux "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/open-dex/conflux-dex-audit/common"
	"github.com/sirupsen/logrus"

	"github.com/spf13/viper"
)

var pow10_10 = big.NewInt(10000000000)
var pow10_12 = big.NewInt(1000000000000)

// Worker auditor
type Worker struct {
	Erc777Map   map[string]*common.Contract
	UsdtAddress string
	Cb          *Config
}

// Start up an auditor
func (w *Worker) Start(wg *sync.WaitGroup, vp *viper.Viper) {
	wg.Add(3)

	go w.runAuditor("BTC", wg, []string{}, vp.GetString("btc.hot"), vp.GetString("btc.cold"))
	go w.runAuditor("ETH", wg, walletAddrs, vp.GetString("eth.hot"))
	go w.runAuditor("USDT", wg, walletAddrs, vp.GetString("usdt.hot"))
}

func (w *Worker) runAuditor(name string, wg *sync.WaitGroup, wallets []string, accounts ...string) {
	defer wg.Done()

	client, err := ethclient.Dial(w.Cb.ETHDial)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "worker",
		}).Warn("Dial ", err.Error())
	}

	usdt, err := BindContract(client, w.UsdtAddress, common.Erc20ABI)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"submodule": "worker",
		}).Warn("BindContract ", err.Error())
	}

	prevCbalance := big.NewInt(0)
	prevBalance := big.NewInt(0)

	for {
		cbalance := w.Erc777Map[name].MustGetTotalSupply()
		balance := big.NewInt(0)

		switch name {
		case "BTC":
			for _, account := range accounts {
				result, err := GetBTCBalance(account)
				if err != nil {
					balance = big.NewInt(0)
					break
				}
				balance.Add(result, balance)
			}
			balance.Mul(pow10_10, balance)
		case "ETH":
			for _, account := range accounts {
				balance.Add(MustGetETHBalance(client, account), balance)
			}

			if balance.Cmp(cbalance) != -1 {
				break
			}

			for _, wallet := range wallets {
				balance.Add(MustGetETHBalance(client, wallet), balance)
				if balance.Cmp(cbalance) != -1 {
					break
				}
			}
		case "USDT":
			for _, account := range accounts {
				balance.Add(MustGetUSDTBalance(client, account, usdt, &bind.CallOpts{}), balance)
			}
			balance.Mul(pow10_12, balance)

			if balance.Cmp(cbalance) != -1 {
				break
			}

			for _, wallet := range wallets {
				balance.Add(new(big.Int).Mul(MustGetUSDTBalance(client, wallet, usdt, &bind.CallOpts{}), pow10_12), balance)
				if balance.Cmp(cbalance) != -1 {
					break
				}
			}
		default:
			logger.WithFields(logrus.Fields{
				"submodule": "worker",
				"name":      name,
			}).Warn("unknown crosschain asset")
		}

		if balance.Cmp(big.NewInt(0)) != 0 && (balance.Cmp(prevBalance) != 0 || cbalance.Cmp(prevCbalance) != 0) {
			if balance.Cmp(cbalance) == -1 {
				err := fmt.Sprintf("%s: %s C%s: %s diff: %s", name, balance.String(), name, cbalance.String(), (new(big.Int).Sub(balance, cbalance)).String())
				common.Alert(module, err)
			}

			logger.WithFields(logrus.Fields{
				"submodule": "worker",
				"name":      name,
				"diff":      new(big.Int).Sub(balance, cbalance),
			}).Info("balance: ", balance, " conflux balance: ", cbalance)

			prevBalance = balance
			prevCbalance = cbalance
		}

		time.Sleep(60 * time.Second)
	}
}
