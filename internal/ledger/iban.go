package ledger

import (
	"math/big"
	"strings"
)

// Big ints used in IBAN validation.
var (
	bigInts36 [36]*big.Int
	big100    = big.NewInt(100)
	big97     = big.NewInt(97)
)

func init() {
	for i := 0; i < len(bigInts36); i++ {
		bigInts36[i] = big.NewInt(int64(i))
	}
}

// ValidIBAN reports whether the given IBAN is valid according to the
// ISO 13616:2020 validation rules. In particular, it checks whether
// the ISO code has the right format and whether the checksum is valid.
func ValidIBAN(iban string) bool {
	if iban == "" {
		return false
	}
	if iban[0] == ' ' || iban[len(iban)-1] == ' ' {
		return false // No whitespace at the beginning or end.
	}
	// Ignore whitespace in the middle.
	iban = strings.ReplaceAll(iban, " ", "")
	// Needs to have ISO code + checksum + at least something.
	if len(iban) < 5 {
		return false
	}
	// Check ISO code and checksum.
	for i := range 4 {
		if !(iban[i] >= 'A' && iban[i] <= 'Z' || i >= 2 && iban[i] >= '0' && iban[i] <= '9') {
			return false
		}
	}
	// Build number for "mod 97" validation.
	k := big.NewInt(0)
	for _, r := range iban[4:] {
		if r >= 'A' && r <= 'Z' {
			k.Mul(k, big100) // Shift left by two digits.
			k.Add(k, bigInts36[int(r-'A')+10])
		} else if r >= '0' && r <= '9' {
			k.Mul(k, bigInts36[10])
			k.Add(k, bigInts36[int(r-'0')])
		} else {
			return false
		}
	}
	// Add the first 4 characters at the end of the validation number.
	for i := range 4 {
		c := iban[i]
		if c >= 'A' && c <= 'Z' {
			k.Mul(k, big100) // Shift left by two digits.
			k.Add(k, bigInts36[int(c-'A')+10])
		} else {
			// Must be a digit, was checked above.
			k.Mul(k, bigInts36[10])
			k.Add(k, bigInts36[int(c-'0')])
		}
	}
	k.Mod(k, big97)
	return k.Cmp(bigInts36[1]) == 0
}
