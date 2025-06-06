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

// Calculates a divided by b, truncated towards zero.
func (a Micros) Div(b Micros) Micros {
	if b == 0 {
		panic("Div: zero divisor")
	}
	// We can't be much faster than Frac's fast path here,
	// so we just re-use the code.
	return a.Frac(UnitValue, b)
}

// Calculates a*numer/denom, truncated towards zero.
// The idea is that this function is defined for more
// inputs than a.Mul(numer).Div(denom) would be as it avoids overflow.
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

// SplitFrac splits m into its integer and fractional parts.
// If m is negative, both parts will have a negative sign,
// unless one of them is zero.
func (m Micros) SplitFrac() (int64, int) {
	return int64(m / 1_000_000), int(m % 1_000_000)
}

func (m Micros) Float() float64 {
	f := float64(m)
	return f / 1e6
}

func FloatAsMicros(f float64) Micros {
	// Note: Min/MaxInt / 1_000_000 can be represented in 44 bits.
	if f < math.MinInt64/1_000_000 || f > math.MaxInt64/1_000_000 {
		panic(fmt.Sprintf("cannot represent %v as Micros", f))
	}
	return Micros(math.Round(f * 1e6))
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

func (m Micros) String() string {
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
		return sign + strconv.FormatInt(i, 10)
	}
	if f%10_000 == 0 {
		return fmt.Sprintf("%s%d.%02d", sign, i, f/10_000)
	}
	return fmt.Sprintf("%s%d.%06d", sign, i, f)
}

// Faster, but uglier.
func (m Micros) String2() string {
	if m == math.MinInt64 {
		return "-9223372036854.775808"
	}
	neg := false
	if m < 0 {
		neg = true
		m = -m
	}
	i, f := m.SplitFrac()
	if f == 0 {
		if neg {
			i = -i
		}
		return strconv.FormatInt(i, 10)
	}

	// Create a buffer to build the string manually
	var buf [32]byte
	pos := len(buf)

	// Fractional part
	if f%10_000 == 0 {
		// Two decimal places
		f /= 10_000
		for j := 0; j < 2; j++ {
			pos--
			buf[pos] = byte(f%10) + '0'
			f /= 10
		}
		pos--
		buf[pos] = '.'
	} else {
		// Six decimal places
		for j := 0; j < 6; j++ {
			pos--
			buf[pos] = byte(f%10) + '0'
			f /= 10
		}
		pos--
		buf[pos] = '.'
	}
	// Integer part
	for i > 0 {
		pos--
		buf[pos] = byte(i%10) + '0'
		i /= 10
	}
	// Prepend sign if needed
	if neg {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}

func (m Micros) MarshalJSON() ([]byte, error) {
	return []byte("\"" + m.String() + "\""), nil
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
