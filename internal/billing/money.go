package billing

import (
	"fmt"
	"math/big"
	"strings"
)

// Money stores exact USD cents. Intermediate calculations use Rat and round
// half up only when an invoice line is materialized.
type Money int64

func ParseMoney(s string) (Money, error) {
	s = strings.TrimSpace(s)
	if !decimalText(s) {
		return 0, fmt.Errorf("invalid decimal amount %q", s)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("invalid decimal amount %q", s)
	}
	cents := new(big.Rat).Mul(r, big.NewRat(100, 1))
	rounded := roundRat(cents)
	if !rounded.IsInt64() {
		return 0, fmt.Errorf("amount out of range")
	}
	return Money(rounded.Int64()), nil
}

func decimalText(s string) bool {
	if s == "" {
		return false
	}
	seenDigit, seenPoint := false, false
	for i, r := range s {
		if (r == '-' || r == '+') && i == 0 {
			continue
		}
		if r >= '0' && r <= '9' {
			seenDigit = true
			continue
		}
		if r == '.' && !seenPoint {
			seenPoint = true
			continue
		}
		return false
	}
	return seenDigit
}
func (m Money) String() string {
	n := big.NewInt(int64(m))
	sign := ""
	if n.Sign() < 0 {
		sign = "-"
		n.Abs(n)
	}
	dollars, cents := new(big.Int), new(big.Int)
	dollars.QuoRem(n, big.NewInt(100), cents)
	return fmt.Sprintf("%s%s.%02d", sign, dollars.String(), cents.Int64())
}
func roundRat(r *big.Rat) *big.Int {
	n := new(big.Int).Set(r.Num())
	d := r.Denom()
	neg := n.Sign() < 0
	if neg {
		n.Neg(n)
	}
	q, rem := new(big.Int).QuoRem(n, d, new(big.Int))
	if new(big.Int).Lsh(rem, 1).Cmp(d) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	return q
}
func decimalRat(s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return nil, fmt.Errorf("invalid nonnegative decimal %q", s)
	}
	return r, nil
}
