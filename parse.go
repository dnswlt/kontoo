package kontoo

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type CommandArgs struct {
	Args        []string
	KeywordArgs map[string][]string
}

type argType int32

const (
	MoneyAmountArg argType = iota
	StringArg
	DecimalArg
	DateArg
	EntryTypeArg
)

type ArgSpec struct {
	Name       string
	Type       argType
	ShortNames []string
}

var allArgs []ArgSpec = []ArgSpec{
	{
		Name:       "Type",
		Type:       EntryTypeArg,
		ShortNames: []string{"t"},
	},
	{
		Name:       "ValueDate",
		Type:       DateArg,
		ShortNames: []string{"vd", "date"},
	},
	{
		Name:       "AssetRef",
		Type:       StringArg,
		ShortNames: []string{"r", "ref"},
	},
	{
		Name:       "Currency",
		Type:       StringArg,
		ShortNames: []string{"c"},
	},
	{
		Name:       "Value",
		Type:       DecimalArg,
		ShortNames: []string{"v"},
	},
}

var keywordRegex = regexp.MustCompile(`^--?([a-zA-Z]\w*)$`)

func ParseArgs(args []string) (CommandArgs, error) {
	res := CommandArgs{
		KeywordArgs: make(map[string][]string),
	}
	lastKeyword := ""
	for _, arg := range args {
		if len(arg) == 0 {
			// Do nothing
		} else if arg == "--" {
			// Switch back to plain arg parsing.
			lastKeyword = ""
		} else if matches := keywordRegex.FindStringSubmatch(arg); matches != nil {
			keyword := matches[1]
			if _, found := res.KeywordArgs[keyword]; found {
				return CommandArgs{}, fmt.Errorf("keyword %q specified multiple times", keyword)
			}
			res.KeywordArgs[keyword] = []string{}
			lastKeyword = keyword
		} else if lastKeyword != "" {
			res.KeywordArgs[lastKeyword] = append(res.KeywordArgs[lastKeyword], arg)
		} else {
			res.Args = append(res.Args, arg)
		}
	}
	return res, nil
}

func ParseDecimalAsMicros(decimal string, m *Micros) error {
	l := len(decimal)
	if l == 0 {
		return fmt.Errorf("cannot parse empty string as micros")
	}
	pos := 0
	sign := int64(1)
	if decimal[pos] == '-' {
		sign = -1
		pos++
	} else if decimal[pos] == '+' {
		pos++
	}
	intStart := pos
	for pos < l && '0' <= decimal[pos] && decimal[pos] <= '9' {
		pos++
	}
	intEnd := pos
	if pos < l {
		if decimal[pos] != '.' {
			return fmt.Errorf("invalid character in decimal: %v", decimal[pos])
		}
		pos++
	}
	fracStart := pos
	for pos < l && '0' <= decimal[pos] && decimal[pos] <= '9' {
		pos++
	}
	if pos < l {
		return fmt.Errorf("invalid character in decimal: %v", decimal[pos])
	}
	if intEnd == intStart && fracStart == l {
		return fmt.Errorf("decimal contains neither integral nor fractional part: %q", decimal)
	}
	fracVal := 0
	if fracStart < l {
		var err error
		fracVal, err = strconv.Atoi(decimal[fracStart:])
		if err != nil {
			return fmt.Errorf("failed to parse fractional part: %w", err)
		}
		fz := 6 - (l - fracStart)
		if fz < 0 {
			return fmt.Errorf("too many fractional digits (max 6): %q", decimal)
		}
		for fz > 0 {
			fracVal *= 10
			fz--
		}
	}
	var intVal int64
	if intEnd-intStart > 0 {
		var err error
		intVal, err = strconv.ParseInt(decimal[intStart:intEnd], 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse integral part: %w", err)
		}
		if intVal >= math.MaxInt64/UnitValue {
			return fmt.Errorf("decimal is too large to represent as micros: %d", intVal)
		}
	}
	*m = Micros(sign * (intVal*UnitValue + int64(fracVal)))
	return nil
}

var decimalRegexp = regexp.MustCompile(`^-?(\d+(\.\d*)?|\.\d+$)`)

func ParseDecimalAsMicrosOld(decimal string, m *Micros) error {
	if matched := decimalRegexp.MatchString(decimal); !matched {
		return fmt.Errorf("not a valid decimal %q", decimal)
	}
	intpart, fracpart, _ := strings.Cut(decimal, ".")
	var v int64
	var err error
	if intpart != "" {
		v, err = strconv.ParseInt(intpart, 10, 64)
		if err != nil {
			return err
		}
		if v > math.MaxInt64/UnitValue {
			return fmt.Errorf("decimal is too large to represent as micros: %d", v)
		}
	}
	if fracpart == "" {
		*m = Micros(v * UnitValue)
		return nil
	}
	if len(fracpart) > 6 {
		return fmt.Errorf("fractional part cannot be represented as micros %q", fracpart)
	}
	dv, err := strconv.Atoi(fracpart)
	if err != nil {
		return err
	}
	*m = Micros(v*UnitValue + int64(dv*int(math.Pow10(6-len(fracpart)))))
	return nil
}

func ParseDecimal(args []string, m *Micros) error {
	if args == nil {
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments for Micros: %v", args)
	}
	return ParseDecimalAsMicros(args[0], m)
}

func ParseDate(args []string, d *Date) error {
	if args == nil {
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments for date: %v", args)
	}
	s := args[0]
	for _, layout := range []string{"2006-01-02", "02.01.2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			*d = Date(t)
			return nil
		}
	}
	return fmt.Errorf("could not parse date %q", s)
}

func ParseEntryType(args []string, e *EntryType) error {
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments for EntryType: %v", args)
	}
	key := strings.ToLower(args[0])
	for name, value := range _EntryType_name_to_values {
		if strings.HasPrefix(strings.ToLower(name), key) {
			*e = EntryType(value)
			return nil
		}
	}
	return fmt.Errorf("cannot parse %q as EntryType", args)
}

func ParseCurrency(args []string, c *Currency) error {
	if args == nil {
		return nil // Ignore empty
	}
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments for Currency: %v", args)
	}
	v := args[0]
	if !slices.Contains([]string{"EUR", "CHF", "USD"}, v) {
		return fmt.Errorf("invalid currency: %v", v)
	}
	*c = Currency(v)
	return nil
}

func ParseString(args []string, s *string) error {
	*s = strings.Join(args, " ")
	return nil
}

func ParseLedgerEntry(args []string) (*LedgerEntry, error) {
	e := &LedgerEntry{}
	ca, err := ParseArgs(args)
	if err != nil {
		return nil, err
	}
	if len(ca.Args) > 0 {
		if len(ca.Args) >= 1 {
			ca.KeywordArgs["Type"] = []string{ca.Args[0]}
		}
		if len(ca.Args) == 2 {
			ca.KeywordArgs["AssetRef"] = []string{ca.Args[1]}
		}
		if len(ca.Args) > 2 {
			return nil, fmt.Errorf("too many non-keyword arguments: %v", ca.Args[2:])
		}
	}
	if err := ParseEntryType(ca.KeywordArgs["Type"], &e.Type); err != nil {
		return nil, err
	}
	ref := ca.KeywordArgs["AssetRef"]
	if len(ref) != 1 {
		return nil, fmt.Errorf("must specify exactly one -AssetRef, got %v", ref)
	}
	e.AssetRef = ref[0]
	if err := ParseCurrency(ca.KeywordArgs["Currency"], &e.Currency); err != nil {
		return nil, err
	}
	if err := ParseDecimal(ca.KeywordArgs["Value"], &e.ValueMicros); err != nil {
		return nil, err
	}
	if err := ParseDecimal(ca.KeywordArgs["NominalValue"], &e.NominalValueMicros); err != nil {
		return nil, err
	}
	if err := ParseDate(ca.KeywordArgs["ValueDate"], &e.ValueDate); err != nil {
		return nil, err
	}
	if args, found := ca.KeywordArgs["Comment"]; found {
		e.Comment = strings.Join(args, " ")
	}
	return e, nil
}
