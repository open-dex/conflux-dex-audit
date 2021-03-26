package common

import (
	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/open-dex/conflux-dex-audit/log"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"time"
)

// MustNewCfx must creates an instance of CFX client.
func MustNewCfx(cfxURL string) *sdk.Client {
	cfx, err := sdk.NewClient(cfxURL, sdk.ClientOption{
		RetryCount:    20,
		RetryInterval: 2 * time.Second,
	})
	if err != nil {
		log.Fatal("failed to create CFX client: %v", err)
	}

	return cfx
}

// GetAssets queries and returns the asset list via Matchflow REST API.
func GetAssets(matchflowURL string) []Asset {
	client := NewClient(matchflowURL)
	return client.GetAssets()
}

// NewLogger creates a new logger with module and optional fields.
func NewLogger(module string, optionalFields ...map[string]interface{}) logrus.FieldLogger {
	entry := logrus.WithField("module", module)

	if len(optionalFields) == 0 {
		return entry
	}

	return entry.WithFields(logrus.Fields(optionalFields[0]))
}

// Mul multiply two decimal and truncate the result to scale 18
func Mul(x, y decimal.Decimal) decimal.Decimal {
	return x.Mul(y).Truncate(18)
}
