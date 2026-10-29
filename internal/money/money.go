// Package money converts the decimal amounts of an order Event into integers in a currency's
// minor unit (ADR 0064). The exponent of a currency (two places for USD, none for JPY) comes
// from golang.org/x/text/currency, never from a table of our own, and an amount with more
// places than the currency has is refused, not rounded.
package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"

	"golang.org/x/text/currency"
)

// MaxMinor is the largest amount accepted, in minor units. It keeps every amount below what a
// float64 holds exactly, so an amount decoded as a float64 still reads back as it was sent.
const MaxMinor = 999_999_999_999_999

// Exponent is the number of decimal places of the currency with the given ISO 4217 code in
// capitals. An unknown code is an error worded for the caller.
func Exponent(code string) (int, error) {
	unit, err := currency.ParseISO(code)
	if err != nil || code != unit.String() {
		return 0, fmt.Errorf("unknown currency code %q", code)
	}
	scale, _ := currency.Standard.Rounding(unit)
	return scale, nil
}

// Minor converts a JSON number (a json.Number, or a float64 that was decoded from one) to the
// minor unit of the currency. The refusals are worded for the sender of the Event.
func Minor(amount any, code string) (int64, error) {
	exp, err := Exponent(code)
	if err != nil {
		return 0, err
	}
	value, err := decimalOf(amount)
	if err != nil {
		return 0, err
	}
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)))
	if !scaled.IsInt() {
		return 0, fmt.Errorf("has more than %d decimal places, which %s allows", exp, code)
	}
	if scaled.Num().Cmp(big.NewInt(MaxMinor)) > 0 {
		return 0, errors.New("is too large")
	}
	return scaled.Num().Int64(), nil
}

// Check refuses what is not an amount in any currency: not a number or negative. How many places
// it may have depends on the currency, which Minor checks.
func Check(amount any) error {
	_, err := decimalOf(amount)
	return err
}

func decimalOf(amount any) (*big.Rat, error) {
	text, err := textOf(amount)
	if err != nil {
		return nil, err
	}
	value, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, errors.New("must be a number")
	}
	if value.Sign() < 0 {
		return nil, errors.New("must not be negative")
	}
	return value, nil
}

func textOf(amount any) (string, error) {
	switch v := amount.(type) {
	case json.Number:
		return v.String(), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", errors.New("must be a number")
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", errors.New("must be a number")
	}
}
