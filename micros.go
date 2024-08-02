package kontoo

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

type Micros int64

// Returns the result of multiplying two values expressed in micros.
// E.g., a == 2_000_000, b == 3_000_000 ==> Mul(a, b) == 6_000_000.
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

func (a Micros) MulTrunc(b Micros) Micros {
	x := float64(a)
	y := float64(b) / 1e6
	return Micros(math.Trunc(x * y))
}

func (m Micros) SplitFrac() (int64, int) {
	return int64(m / 1_000_000), int(m % 1_000_000)
}

func (m Micros) Format(format string) string {
	// #'##0.00
	// #'##0.00;(#'##0.00)
	// 0.000
	// 0.00%
	// Simpler: let's not invent or implement a formatting language. Use dedicated methods:
	// Format(decimalPlaces int, thousandSeparator rune)
	// Format(decimalPlaces int, thousandSeparator rune, negInBrackets bool)
	// Format(decimalPlaces int, thousandSeparator rune, negInBrackets bool, percent bool)
	//
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
	if i < len(rs) && rs[i] != '.' {
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
			return fmt.Sprintf(`"%s%d.%02d"`, sign, i, f)
		}
		return fmt.Sprintf(`"%s%d.%06d"`, sign, i, f)
	}()
	return []byte(s), nil
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
