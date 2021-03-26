package common

import "math/big"

// Public constants in Conflux or DEX.
const (
	CfxURLMainNet      = "http://mainnet-jsonrpc.conflux-chain.org:12537"
	MatchflowURLTest   = "https://dev.matchflow.io"
	ShuttleflowURLTest = "https://dev.shuttleflow.io"

	ZeroAddress = "0x0000000000000000000000000000000000000000"

	EventHashTransfer = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	EventHashDeposit  = "0x5548c837ab068cf56a2c2479df0882a4922fd203edb7517321831d95078c5f62"
	EventHashWithdraw = "0x9b1bfa7fa9ee420a16e124f794c35ac9f90472acc99140eb2f6447c714cad8eb"
	EventHashWrite    = "0x3802ba8117dc6bf5de2a857e440679e83bb0f9b68094ede7167234a5f97319ed"
)

func GetNetworkId() uint32 {
	return 1029
}

// NumEpochsConfirmed is the number of epochs before latest state that treated as confirmed.
var NumEpochsConfirmed *big.Int = big.NewInt(10)

// DexAdmin is the conflux address of admin of dex
var DexAdmin = "0x0000000000000000000000000000000000000000"

// CustodianAddress is the machine address where custodian node running on
var CustodianAddress = ""

// MatchflowURL is the url of matchflow rest api
var MatchflowURL = ""

// DexAdminPrivKey the private key of dex admin
var DexAdminPrivKey = ""

// AesSecret the password of aes encrypt
var AesSecret = "123456789abcfake"

// Common values often used
var (
	Big0 = big.NewInt(0)
	Big1 = big.NewInt(1)
)
