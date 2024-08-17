package kontoo

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

type Micros int64

var (
	bigMillion = big.NewInt(1_000_000)
)

// Returns the result of multiplying two values expressed in micros.
// E.g., a == 2_000_000, b == 3_000_000 ==> Mul(a, b) == 6_000_000.
func (a Micros) Mul(b Micros) Micros {
	// Fast path, using a simple overflow check.
	if a == 0 {
		return 0
	}
	c := a * b
	if c/a == b {
		return c / 1_000_000
	}
	// Slow path
	bigA := big.NewInt(int64(a))
	bigB := big.NewInt(int64(b))
	bigA.Mul(bigA, bigB)
	bigA.Quo(bigA, bigMillion)
	if !bigA.IsInt64() {
		panic(fmt.Sprintf("Mul: cannot represent %v as int64 micros", bigA))
	}
	return Micros(bigA.Int64())
}

// Calculates the fraction numer/denom of this Micros value.
func (a Micros) Frac(numer, denom Micros) Micros {
	if denom == 0 {
		panic("Frac: zero denominator")
	}
	// Fast path: a * numer can be represented
	if a == 0 {
		return 0
	}
	c := a * numer
	if c/a == numer {
		return c / denom
	}
	// Slow path.
	bigA := big.NewInt(int64(a))
	bigN := big.NewInt(int64(numer))
	bigD := big.NewInt(int64(denom))
	bigA.Mul(bigA, bigN)
	bigA.Quo(bigA, bigD)
	if !bigA.IsInt64() {
		panic(fmt.Sprintf("Frac: cannot represent %v as int64 micros", bigA))
	}
	return Micros(bigA.Int64())
}

func (m Micros) SplitFrac() (int64, int) {
	return int64(m / 1_000_000), int(m % 1_000_000)
}

func (m Micros) Format(format string) string {
	// "()'.3"  "-.3"  ".3%"
	// Must be a format string
	rs := []rune(format)
	i := 0
	negBrackets := false
	if len(rs) >= 2 && rs[0] == '(' && rs[1] == ')' {
		i += 2
		negBrackets = true
	} else if len(rs) >= 1 && rs[0] == '-' {
		i++
	}
	thousandSep := ""
	if i < len(rs) && rs[i] != '.' && rs[i] != '%' {
		thousandSep = string(rs[i])
		i++
	}
	decimalPlaces := -1 // -1 means "as many as needed"
	if i < len(rs) && rs[i] == '.' {
		i++
		if i < len(rs) && rs[i] >= '0' && rs[i] <= '9' {
			decimalPlaces = int(rs[i] - '0')
			if decimalPlaces > 6 {
				return "INVALID_FORMAT(" + format + ")"
			}
			i++
		}
	}
	usePercent := false
	if i < len(rs) && rs[i] == '%' {
		usePercent = true
		m = m.Mul(100 * UnitValue)
		i++
	}
	if i < len(rs) {
		return "INVALID_FORMAT(" + format + ")"
	}
	j, f := m.SplitFrac()
	var sb strings.Builder
	if m < 0 {
		if negBrackets {
			sb.WriteRune('(')
		} else {
			sb.WriteRune('-')
		}
		j, f = -j, -f
	}
	// Integer part
	if thousandSep != "" && j > 999 {
		var parts [7]string
		k := len(parts) - 1
		jp := j
		for jp > 999 {
			parts[k] = strconv.FormatInt(jp%1000, 10)
			switch len(parts[k]) {
			case 2:
				parts[k] = "0" + parts[k]
			case 1:
				parts[k] = "00" + parts[k]
			}
			jp /= 1000
			k--
		}
		parts[k] = strconv.Itoa(int(jp))
		sb.WriteString(strings.Join(parts[k:], thousandSep))
	} else {
		sb.WriteString(strconv.FormatInt(j, 10))
	}
	// Decimal part
	if decimalPlaces > 0 {
		sb.WriteRune('.')
		fs := fmt.Sprintf("%06d", f)
		sb.WriteString(fs[:decimalPlaces])
	} else if decimalPlaces == -1 && f > 0 {
		sb.WriteRune('.')
		fs := fmt.Sprintf("%06d", f)
		dp := 6
		for i := 5; i >= 0 && fs[i] == '0'; i-- {
			dp--
		}
		sb.WriteString(fs[:dp])
	}
	if usePercent {
		sb.WriteRune('%')
	}
	if m < 0 && negBrackets {
		sb.WriteRune(')')
	}
	return sb.String()
}

func (m Micros) MarshalJSON() ([]byte, error) {
	s := func() string {
		if m == math.MinInt64 {
			return `"-9223372036854.775808"`
		}
		sign := ""
		if m < 0 {
			sign = "-"
			m = -m
		}
		i, f := m.SplitFrac()
		if f == 0 {
			return fmt.Sprintf(`"%s%d"`, sign, i)
		}
		if f%10_000 == 0 {
			return fmt.Sprintf(`"%s%d.%02d"`, sign, i, f/10_000)
		}
		return fmt.Sprintf(`"%s%d.%06d"`, sign, i, f)
	}()
	return []byte(s), nil
}

func (m *Micros) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return ParseDecimalAsMicros(s[1:len(s)-1], m)
	}
	d, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid JSON string for Micros: %s", s)
	}
	*m = Micros(d)
	return nil
}
