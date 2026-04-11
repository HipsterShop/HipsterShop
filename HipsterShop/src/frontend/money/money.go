//
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package money

import (
	"errors"
)

const (
	nanosMin = -999999999
	nanosMax = +999999999
	nanosMod = 1000000000
)

var (
	ErrInvalidValue        = errors.New("one of the specified money values is invalid")
	ErrMismatchingCurrency = errors.New("mismatching currency codes")
)

// Money represents a monetary value.
type Money struct {
	CurrencyCode string `json:"currencyCode"`
	Units        int64  `json:"units"`
	Nanos        int32  `json:"nanos"`
}

// IsValid checks if specified value has a valid units/nanos signs and ranges.
func IsValid(m Money) bool {
	return signMatches(m) && validNanos(m.Nanos)
}

func signMatches(m Money) bool {
	return m.Nanos == 0 || m.Units == 0 || (m.Nanos < 0) == (m.Units < 0)
}

func validNanos(nanos int32) bool { return nanosMin <= nanos && nanos <= nanosMax }

// IsZero returns true if the specified money value is equal to zero.
func IsZero(m Money) bool { return m.Units == 0 && m.Nanos == 0 }

// IsPositive returns true if the specified money value is valid and is
// positive.
func IsPositive(m Money) bool {
	return IsValid(m) && m.Units > 0 || (m.Units == 0 && m.Nanos > 0)
}

// IsNegative returns true if the specified money value is valid and is
// negative.
func IsNegative(m Money) bool {
	return IsValid(m) && m.Units < 0 || (m.Units == 0 && m.Nanos < 0)
}

// AreSameCurrency returns true if values l and r have a currency code and
// they are the same values.
func AreSameCurrency(l, r Money) bool {
	return l.CurrencyCode == r.CurrencyCode && l.CurrencyCode != ""
}

// AreEquals returns true if values l and r are the equal, including the
// currency. This does not check validity of the provided values.
func AreEquals(l, r Money) bool {
	return l.CurrencyCode == r.CurrencyCode &&
		l.Units == r.Units && l.Nanos == r.Nanos
}

// Negate returns the same amount with the sign negated.
func Negate(m Money) Money {
	return Money{
		Units:        -m.Units,
		Nanos:        -m.Nanos,
		CurrencyCode: m.CurrencyCode}
}

// Must panics if the given error is not nil.
func Must(v Money, err error) Money {
	if err != nil {
		panic(err)
	}
	return v
}

// Sum adds two values.
func Sum(l, r Money) (Money, error) {
	if !IsValid(l) || !IsValid(r) {
		return Money{}, ErrInvalidValue
	} else if l.CurrencyCode != r.CurrencyCode {
		return Money{}, ErrMismatchingCurrency
	}
	units := l.Units + r.Units
	nanos := l.Nanos + r.Nanos

	if (units == 0 && nanos == 0) || (units > 0 && nanos >= 0) || (units < 0 && nanos <= 0) {
		units += int64(nanos / nanosMod)
		nanos = nanos % nanosMod
	} else {
		if units > 0 {
			units--
			nanos += nanosMod
		} else {
			units++
			nanos -= nanosMod
		}
	}

	return Money{
		Units:        units,
		Nanos:        nanos,
		CurrencyCode: l.CurrencyCode}, nil
}

// MultiplySlow is a slow multiplication operation done through adding the value
// to itself n-1 times.
func MultiplySlow(m Money, n uint32) Money {
	out := m
	for n > 1 {
		out = Must(Sum(out, m))
		n--
	}
	return out
}
