package kontoo

import (
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNoInput        = errors.New("no input")
	ErrInvalidCommand = errors.New("invalid command")
)

type Cmd interface{}

type CmdAdd struct {
	update LedgerUpdate
}

type CmdHelp struct{}

type fpParams struct {
	txType             EntryType
	valueDate          time.Time
	assetId            string
	currency           Currency
	priceMicros        int64
	quantityMicros     int64
	nominalValueMicros int64
	valueMicros        int64
	costMicros         int64
}

func ParseDecimalAsMicros(decimal string) (int64, error) {
	if matched, _ := regexp.MatchString(`\d+(\.\d*)?|\.\d+`, decimal); !matched {
		return 0, fmt.Errorf("not a valid decimal %q", decimal)
	}
	intpart, fracpart, _ := strings.Cut(decimal, ".")
	var v int64
	var err error
	if intpart != "" {
		v, err = strconv.ParseInt(intpart, 10, 64)
		if err != nil {
			return 0, err
		}
		if v > math.MaxInt64/UnitValue {
			return 0, fmt.Errorf("decimal is too large to represent as micros: %d", v)
		}
	}
	if fracpart == "" {
		return v * UnitValue, nil
	}
	if len(fracpart) > 6 {
		return 0, fmt.Errorf("fractional part cannot be represented as micros %q", fracpart)
	}
	dv, err := strconv.Atoi(fracpart)
	if err != nil {
		return 0, err
	}
	return v*UnitValue + int64(dv*int(math.Pow10(6-len(fracpart)))), nil
}

func trySetKeylessParam(param string, fp *fpParams) bool {
	// Try to parse as EntryType.
	txMap := map[string]EntryType{
		"buy":  BuyTransaction,
		"sell": SellTransaction,
	}
	if t, ok := txMap[param]; ok {
		fp.txType = t
		return true
	}
	// Try to parse as time.
	for _, layout := range []string{"2006-01-02", "02.01.2006"} {
		if t, err := time.Parse(layout, param); err == nil {
			fp.valueDate = t
			return true
		}
	}
	// Nothing matched.
	return false
}

func trySetKeyedParam(key string, val string, fp *fpParams) error {
	key = strings.ToLower(key)
	tryParse := func(i *int64) error {
		v, err := ParseDecimalAsMicros(val)
		if err != nil {
			return err
		}
		*i = v
		return nil
	}
	if strings.HasPrefix("quantity", key) || strings.HasPrefix("number", key) {
		return tryParse(&fp.quantityMicros)
	}
	if strings.HasPrefix("price", key) {
		return tryParse(&fp.priceMicros)
	}
	if strings.HasPrefix("value", key) {
		return tryParse(&fp.valueMicros)
	}
	if strings.HasPrefix("cost", key) {
		return tryParse(&fp.costMicros)
	}
	if strings.HasPrefix("nominal", key) {
		return tryParse(&fp.nominalValueMicros)
	}
	if strings.HasPrefix("currency", key) {
		if matched, _ := regexp.MatchString(`[A-Z]{3}`, val); !matched {
			return fmt.Errorf("invalid currency %q (expected three uppercase letters)", val)
		}
		fp.currency = Currency(val)
		return nil
	}
	if key == "id" {
		fp.assetId = val
		return nil
	}
	return fmt.Errorf("unknown key %q", key)
}

func fuzzyParseParams(params []string) (fpParams, error) {
	fp := fpParams{}
	for _, p := range params {
		k, v, cut := strings.Cut(p, ":")
		if !cut {
			if !trySetKeylessParam(p, &fp) {
				return fpParams{}, fmt.Errorf("couldn't parse input %q", p)
			}
			continue
		}
		if err := trySetKeyedParam(k, v, &fp); err != nil {
			return fpParams{}, fmt.Errorf("couldn't parse keyed input %q: %s", p, err)
		}
	}
	return fp, nil
}

func (cli *CLI) processAdd(params []string) {
}

type CLI struct {
	ledgerPath string
	ledger     *Ledger
}

func NewCLI(ledgerPath string, createIfNotFound bool) (*CLI, error) {
	l := NewLedger()
	if _, err := os.Stat(ledgerPath); errors.Is(err, os.ErrNotExist) {
		if !createIfNotFound {
			return nil, fmt.Errorf("path %s does not exist", ledgerPath)
		}
	} else if err := l.Load(ledgerPath); err != nil {
		return nil, err
	}
	return &CLI{
		ledgerPath: ledgerPath,
		ledger:     l,
	}, nil
}

func (cli *CLI) processSave() {
	err := cli.ledger.Save(cli.ledgerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not save ledger: %s\n", err)
	}
}

func (cli *CLI) ProcessLine(line string) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return // Skip
	}
	switch strings.ToLower(fields[0]) {
	case "add":
		cli.processAdd(fields[1:])
	case "save":
		cli.processSave()
	case "help":
		fmt.Println("Help not yet implemented")
	default:
		fmt.Println("Could not parse input. Try 'help'.")
	}
}
