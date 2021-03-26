package common

import (
	"fmt"
	"math/big"
	"strings"
)

// AccountBalances represents a collection of accounts with their balances.
// Note, the account address is case-insensitive to avoid BIP-155 address.
type AccountBalances struct {
	items map[string]*big.Int
}

// NewAccountBalances creates an instance of AccountBalances.
func NewAccountBalances() *AccountBalances {
	return &AccountBalances{
		items: make(map[string]*big.Int),
	}
}

// GoString implements the fmt.Stringer interface.
func (ab AccountBalances) String() string {
	return fmt.Sprintf("%v", ab.items)
}

// GoString implements the fmt.GoStringer interface.
func (ab AccountBalances) GoString() string {
	return fmt.Sprintf("%#v", ab.items)
}

// Get returns the balance of specified account if exists.
// Otherwise, return zero.
func (ab *AccountBalances) Get(account string) *big.Int {
	account = strings.ToLower(account)

	balance, ok := ab.items[account]
	if ok {
		return balance
	}

	return Big0
}

// Add adds a new account with amount as balance if not exists.
// Otherwise, add amount to the existing balance.
func (ab *AccountBalances) Add(account string, amount *big.Int) {
	account = strings.ToLower(account)

	current, ok := ab.items[account]

	if ok {
		ab.items[account] = new(big.Int).Add(current, amount)
	} else {
		ab.items[account] = amount
	}
}

// Map returns the underlying account balances data structure.
// Generally, it is used for iteration purpose, and should never
// change the data outside of AccountBalances.
func (ab *AccountBalances) Map() map[string]*big.Int {
	return ab.items
}

// Sum calculates the sum of balances for all accounts.
func (ab *AccountBalances) Sum() *big.Int {
	sum := Big0

	for _, balance := range ab.items {
		sum = new(big.Int).Add(sum, balance)
	}

	return sum
}
