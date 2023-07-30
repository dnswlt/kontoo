package kontoo

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
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

type CLIConfig struct {
	CreateIfNotFound bool
	LedgerPath       string
	AssetListPath    string
	DefaultCurrency  string
}

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

func tryParseDate(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", "02.01.2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
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
	if valueDate, ok := tryParseDate(param); ok {
		fp.valueDate = valueDate
		return true
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
	// Keep sorted, except in the case of common prefixes:
	// put the key that should take preference first.
	if strings.HasPrefix("cost", key) {
		return tryParse(&fp.costMicros)
	}
	if strings.HasPrefix("currency", key) {
		if matched, _ := regexp.MatchString(`[A-Z]{3}`, val); !matched {
			return fmt.Errorf("invalid currency %q (expected three uppercase letters)", val)
		}
		fp.currency = Currency(val)
		return nil
	}
	if strings.HasPrefix("date", key) {
		d, ok := tryParseDate(val)
		if !ok {
			return fmt.Errorf("invalid value date: %s", val)
		}
		fp.valueDate = d
		return nil
	}
	if key == "id" {
		fp.assetId = val
		return nil
	}
	if strings.HasPrefix("nominal", key) {
		return tryParse(&fp.nominalValueMicros)
	}
	if strings.HasPrefix("price", key) {
		return tryParse(&fp.priceMicros)
	}
	if strings.HasPrefix("quantity", key) || strings.HasPrefix("number", key) {
		return tryParse(&fp.quantityMicros)
	}
	if strings.HasPrefix("value", key) {
		return tryParse(&fp.valueMicros)
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

type CLI struct {
	config *CLIConfig
	ledger *Ledger
	assets []*Asset // sorted by ToUpper(Id).
}

func loadAssetList(path string) ([]*Asset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var assetList []*Asset
	if err := json.Unmarshal(data, &assetList); err != nil {
		return nil, err
	}
	sort.Slice(assetList, func(i, j int) bool {
		return strings.ToUpper(assetList[i].Id) < strings.ToUpper(assetList[j].Id)
	})
	return assetList, nil
}

func NewCLI(cfg *CLIConfig) (*CLI, error) {
	l := NewLedger()
	if _, err := os.Stat(cfg.LedgerPath); errors.Is(err, os.ErrNotExist) {
		if !cfg.CreateIfNotFound {
			return nil, fmt.Errorf("path %s does not exist", cfg.LedgerPath)
		}
	} else if err := l.Load(cfg.LedgerPath); err != nil {
		return nil, err
	}
	assets := []*Asset{}
	if cfg.AssetListPath != "" {
		var err error
		assets, err = loadAssetList(cfg.AssetListPath)
		if err != nil {
			return nil, err
		}
	}
	return &CLI{
		config: cfg,
		ledger: l,
		assets: assets,
	}, nil
}

func (cli *CLI) findAssetById(id string) *Asset {
	id = strings.ToUpper(id)
	i := sort.Search(len(cli.assets), func(i int) bool {
		return strings.ToUpper(cli.assets[i].Id) >= id
	})
	j := i
	for j <= i+1 && j < len(cli.assets) && strings.HasPrefix(strings.ToUpper(cli.assets[j].Id), id) {
		j++
	}
	if j != i+1 {
		// No unique asset
		return nil
	}
	return cli.assets[i]
}

var (
	firstValidTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
)

func (cli *CLI) processAdd(params []string) {
	fp, err := fuzzyParseParams(params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "add: %s\n", err)
		return
	}
	switch fp.txType {
	case BuyTransaction, SellTransaction:
		if fp.valueDate.Before(firstValidTime) {
			fmt.Printf("invalid value date: %v\n", fp.valueDate)
			return
		}
		asset := cli.findAssetById(fp.assetId)
		if asset == nil {
			fmt.Printf("unknown asset ID: %s\n", fp.assetId)
			return
		}
		params := &TransactionParams{
			txType:             fp.txType,
			valueDate:          fp.valueDate,
			asset:              *asset,
			currency:           fp.currency,
			priceMicros:        fp.priceMicros,
			quantityMicros:     fp.quantityMicros,
			nominalValueMicros: fp.nominalValueMicros,
			valueMicros:        fp.valueMicros,
			costMicros:         fp.costMicros,
		}
		if err := ValidateTransactionParams(params); err != nil {
			fmt.Printf("invalid transaction: %s\n", err)
			return
		}
		e, err := cli.ledger.AddTransaction(params)
		if err != nil {
			fmt.Printf("failed to add stock transaction: %s\n", err)
			return
		}
		fmt.Printf("Added new ledger entry: %v\n", e)
	default:
		fmt.Printf("transaction type %v not supported\n", fp.txType)
	}
}

func (cli *CLI) processList() {
	if len(cli.ledger.entries) == 0 {
		fmt.Printf("empty ledger\n")
	}
	for _, e := range cli.ledger.entries {
		fmt.Printf("%s\n", FormatEntry(e, ""))
	}
}

func (cli *CLI) processSave() {
	err := cli.ledger.Save(cli.config.LedgerPath)
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
	case "list", "ls":
		cli.processList()
	case "save":
		cli.processSave()
	case "help":
		fmt.Println("Help not yet implemented")
	default:
		fmt.Println("Could not parse input. Try 'help'.")
	}
}
