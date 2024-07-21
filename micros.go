package kontoo

import (
	"fmt"
	"math/big"
)

type Micros int64

// Returns the result of multiplying two values expressed in micros.
// E.g., a == 2_000_000, b == 3_000_000 ==> MultMicros(a, b) == 6_000_000.
func (a Micros) Mul(b Micros) Micros {
	bigA := big.NewInt(int64(a))
	bigB := big.NewInt(int64(b))
	bigA.Mul(bigA, bigB)
	bigA.Div(bigA, big.NewInt(1_000_000))
	if !bigA.IsInt64() {
		panic(fmt.Sprintf("cannot represent %v as int64 micros", bigA))
	}
	return Micros(bigA.Int64())
}

func (m Micros) Format() string {
	sign := ""
	if m < 0 {
		sign = "-"
		m = -m
	}
	frac := m % 1_000_000
	if frac == 0 {
		return fmt.Sprintf(`"%s%d"`, sign, m/1_000_000)
	}
	if frac%10_000 == 0 {
		return fmt.Sprintf(`"%s%d.%02d"`, sign, m/1_000_000, frac/10_000)
	}
	return fmt.Sprintf(`"%s%d.%06d"`, sign, m/1_000_000, m%1_000_000)
}

func (m Micros) MarshalJSON() ([]byte, error) {
	return []byte(m.Format()), nil
}

func (m *Micros) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" {
		return nil // Default behaviour for JSON unmarshalling of 'null'.
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("invalid JSON string for Micros: %s", s)
	}
	return ParseDecimalAsMicros(s[1:len(data)-1], m)
}
