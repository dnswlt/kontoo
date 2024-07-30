package kontoo

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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
			r, _ := utf8.DecodeRuneInString(decimal[pos:])
			return fmt.Errorf("invalid character in decimal: %c", r)
		}
		pos++
	}
	fracStart := pos
	for pos < l && '0' <= decimal[pos] && decimal[pos] <= '9' {
		pos++
	}
	if pos < l {
		r, _ := utf8.DecodeRuneInString(decimal[pos:])
		return fmt.Errorf("invalid character in decimal: %c", r)
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

func ParseDecimal(args []string, m *Micros) error {
	if args == nil {
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments for Micros: %v", args)
	}
	return ParseDecimalAsMicros(args[0], m)
}

func ParseDecimalOrPercent(args []string, m *Micros) error {
	if args == nil {
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments for Micros: %v", args)
	}
	arg := args[0]
	isPercent := false
	if strings.HasSuffix(arg, "%") {
		arg = strings.TrimSuffix(arg, "%")
		isPercent = true
	}
	if err := ParseDecimalAsMicros(arg, m); err != nil {
		return err
	}
	if isPercent {
		*m = m.Mul(10 * Millis)
	}
	return nil
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
			*d = Date{t}
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
	for _, typ := range EntryTypeValues() {
		name := typ.String()
		if strings.HasPrefix(strings.ToLower(name), key) {
			*e = typ
			return nil
		}
	}
	return fmt.Errorf("cannot parse %q as EntryType", args)
}

func ParseAssetType(args []string, e *AssetType) error {
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments for AssetType: %v", args)
	}
	key := strings.ToLower(args[0])
	for _, typ := range AssetTypeValues() {
		name := typ.String()
		if strings.HasPrefix(strings.ToLower(name), key) {
			*e = typ
			return nil
		}
	}
	return fmt.Errorf("cannot parse %q as AssetType", args)
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

type argParseFunc = func([]string) error

type argSpec struct {
	args map[string]argParseFunc
	// pArgs lists args that can also be specified without a keyword
	// (i.e., as positional arguments), in order. The last of these will receive
	// all trailing arguments.
	pArgs []string
}

func NewArgSpec() *argSpec {
	return &argSpec{
		args: make(map[string]argParseFunc),
	}
}

func (s *argSpec) entryType(name string, e *EntryType) {
	s.args[name] = func(xs []string) error {
		return ParseEntryType(xs, e)
	}
}

func (s *argSpec) assetType(name string, e *AssetType) {
	s.args[name] = func(xs []string) error {
		return ParseAssetType(xs, e)
	}
}

func (s *argSpec) currency(name string, c *Currency) {
	s.args[name] = func(xs []string) error {
		return ParseCurrency(xs, c)
	}
}

func (s *argSpec) decimal(name string, d *Micros) {
	s.args[name] = func(xs []string) error {
		return ParseDecimal(xs, d)
	}
}

func (s *argSpec) decimalOrPercent(name string, d *Micros) {
	s.args[name] = func(xs []string) error {
		return ParseDecimalOrPercent(xs, d)
	}
}

func (s *argSpec) date(name string, d *Date) {
	s.args[name] = func(xs []string) error {
		return ParseDate(xs, d)
	}
}

func (s *argSpec) datePtr(name string, d **Date) {
	s.args[name] = func(xs []string) error {
		t := new(Date)
		if err := ParseDate(xs, t); err != nil {
			return err
		}
		*d = t
		return nil
	}
}

func (s *argSpec) strings(name, sep string, out *string) {
	s.args[name] = func(xs []string) error {
		*out = strings.Join(xs, sep)
		return nil
	}
}

func (s *argSpec) str(name string, out *string) {
	s.args[name] = func(xs []string) error {
		if len(xs) != 1 {
			return fmt.Errorf("too many arguments (%d) for string %q", len(xs), name)
		}
		*out = xs[0]
		return nil
	}
}

func subseq(s, t string) bool {
	if len(s) < len(t) {
		return false
	}
	sp := 0
	for i := 0; i < len(t); i++ {
		for sp < len(s) && s[sp] != t[i] {
			sp++
		}
		if sp == len(s) {
			return false
		}
		sp++
	}
	return true
}

// matchArg finds the unique argument in argSpec that has sub as a subsequence.
// If no unique such argument exists, it returns false.
func (s *argSpec) matchArg(sub string) (string, bool) {
	if _, ok := s.args[sub]; ok {
		// Exact match always comes first.
		return sub, true
	}
	// Try case-insensitive subsequence match.
	sub = strings.ToLower(sub)
	match, found := "", false
	for a := range s.args {
		if subseq(strings.ToLower(a), sub) {
			if found {
				return "", false // not unique
			}
			match, found = a, true
		}
	}
	return match, found
}

func (s *argSpec) parse(args []string) error {
	ca, err := ParseArgs(args)
	if err != nil {
		return err
	}
	for name, args := range ca.KeywordArgs {
		if fullName, ok := s.matchArg(name); ok {
			err := s.args[fullName](args)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("invalid argument: %q", name)
		}
	}
	// Parse positional args.
	if len(ca.Args) > 0 {
		np := len(s.pArgs)
		if np == 0 {
			return fmt.Errorf("excess positional arguments: %v", ca.Args)
		}
		i := 0
		for i < len(ca.Args) && i < np-1 {
			if err := s.args[s.pArgs[i]](ca.Args[i : i+1]); err != nil {
				return err
			}
			i++
		}
		// Excess args
		if i < len(ca.Args) {
			if err := s.args[s.pArgs[i]](ca.Args[i:]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *argSpec) positionalArgs(names ...string) {
	s.pArgs = append(s.pArgs, names...)
}

func ledgerEntryArgSpec(e *LedgerEntry) *argSpec {
	s := NewArgSpec()
	s.entryType("Type", &e.Type)
	s.str("AssetID", &e.AssetID)
	s.str("AssetRef", &e.AssetRef)
	s.currency("Currency", &e.Currency)
	s.currency("QuoteCurrency", &e.QuoteCurrency)
	s.decimal("Value", &e.ValueMicros)
	s.decimal("Cost", &e.CostMicros)
	s.decimal("Quantity", &e.QuantityMicros)
	s.decimal("Price", &e.PriceMicros)
	s.date("ValueDate", &e.ValueDate)
	s.strings("Comment", " ", &e.Comment)
	s.positionalArgs("Type", "AssetRef")
	return s
}

func assetArgSpec(a *Asset) *argSpec {
	s := NewArgSpec()
	s.assetType("Type", &a.Type)
	s.str("Name", &a.Name)
	s.str("ShortName", &a.ShortName)
	s.datePtr("IssueDate", &a.IssueDate)
	s.datePtr("MaturityDate", &a.MaturityDate)
	s.decimalOrPercent("Interest", &a.InterestMicros)
	s.str("IBAN", &a.IBAN)
	s.str("AccountNumber", &a.AccountNumber)
	s.str("ISIN", &a.ISIN)
	s.str("WKN", &a.WKN)
	s.str("TickerSymbol", &a.TickerSymbol)
	s.str("CustomID", &a.CustomID)
	s.currency("Currency", &a.Currency)
	s.str("AssetGroup", &a.AssetGroup)
	s.strings("Comment", " ", &a.Comment)
	return s
}

func ParseLedgerEntry(args []string) (*LedgerEntry, error) {
	e := &LedgerEntry{}
	s := ledgerEntryArgSpec(e)
	err := s.parse(args)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func ParseAsset(args []string) (*Asset, error) {
	a := &Asset{}
	s := assetArgSpec(a)
	err := s.parse(args)
	if err != nil {
		return nil, err
	}
	return a, nil
}
