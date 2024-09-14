package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dnswlt/kontoo"
)

func ProcessAdd(args []string) error {
	path := "./ledger.json"
	store, err := kontoo.LoadStore(path)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	e, err := kontoo.ParseLedgerEntry(args)
	if err != nil {
		return fmt.Errorf("could not parse ledger entry: %w", err)
	}
	err = store.Add(e)
	if err != nil {
		return fmt.Errorf("could not add entry: %w", err)
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	store.Save()

	return nil
}

func ProcessServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 8084, "The port on which to listen")
	ledgerPath := fs.String("ledger", "./ledger.json", "Path to the ledger.json file")
	baseDir := fs.String("base-dir", ".", "Base directory from which static resources are served")
	debugMode := fs.Bool("debug", false, "Enable debug mode")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse error for serve flags: %w", err)
	}
	s, err := kontoo.NewServer(fmt.Sprintf("localhost:%d", *port), *ledgerPath, *baseDir)
	if err != nil {
		return err
	}
	s.DebugMode(*debugMode)
	return s.Serve()
}

func ProcessImport(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("please specify ledger and one CSV file. Got: %v", args)
	}
	store, err := kontoo.LoadStore(args[0])
	if err != nil {
		return fmt.Errorf("cannot load store: %v", err)
	}
	in, err := os.Open(args[1])
	if err != nil {
		return fmt.Errorf("cannot open CSV file: %v", err)
	}
	defer in.Close()
	parseFloat := func(s string) (kontoo.Micros, error) {
		s = strings.TrimSpace(strings.ReplaceAll(s, "'", ""))
		if s == "" {
			return 0, nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, err
		}
		return kontoo.Micros(math.Round(f * 1e6)), nil
	}
	colIdx := map[string]int{
		"ValueDate": 0,
		"EntryType": 1,
		"AssetID":   2,
		"Currency":  5,
		"Value":     6,
		"Cost":      7,
		"Quantity":  8,
		"Price":     9,
	}
	r := csv.NewReader(in)
	firstRow := true
	ccyRe := regexp.MustCompile("^[A-Z]{3}$")
	assetIDs := map[string]bool{}
	nImported := 0
	for i := 0; ; i++ {
		row, err := r.Read()
		if err == io.EOF || errors.Is(err, csv.ErrFieldCount) {
			break
		}
		if firstRow {
			firstRow = false
			continue
		}
		if row[colIdx["EntryType"]] != "Kauf" {
			// Only AssetPurchase
			continue
		}
		assetId := row[colIdx["AssetID"]]
		if strings.HasPrefix(assetId, "DE") || strings.HasPrefix(assetId, "NL") {
			// Skip these assets, they are already imported or no longer relevant.
			continue
		}
		ccy := row[colIdx["Currency"]]
		if !ccyRe.MatchString(ccy) {
			return fmt.Errorf("invalid currency in row %d: %s", i, ccy)
		}
		valueDate, err := time.Parse("02.01.2006", row[colIdx["ValueDate"]])
		if err != nil {
			return fmt.Errorf("invalid date in row %d: %q", i, strings.Join(row, ","))
		}
		val, err := parseFloat(row[colIdx["Value"]])
		if err != nil {
			return fmt.Errorf("invalid value in row %d: %w", i, err)
		}
		qty, err := parseFloat(row[colIdx["Quantity"]])
		if err != nil {
			return fmt.Errorf("invalid quantity in row %d: %w", i, err)
		}
		price, err := parseFloat(row[colIdx["Price"]])
		if err != nil {
			return fmt.Errorf("invalid price in row %d: %w", i, err)
		}
		cost, err := parseFloat(row[colIdx["Cost"]])
		if err != nil {
			return fmt.Errorf("invalid cost in row %d: %w", i, err)
		}
		if qty == 0 || val == 0 || price == 0 {
			return fmt.Errorf("zero values in row %d: %v %v %v", i, qty, val, price)
		}
		if eps := math.Abs(1 - qty.Mul(price).Float()/val.Float()); eps >= 0.02 {
			// see if price is given in %
			price = price.Mul(10 * kontoo.Millis)
			if eps := math.Abs(1 - qty.Mul(price).Float()/val.Float()); eps >= 0.02 {
				return fmt.Errorf("price*qty != value in row %d: %.2f %v %v %v", i, eps, qty, val, price)
			}
		}
		vdate := kontoo.Date{Time: valueDate}
		if entries := store.EntriesInRange(assetId, vdate, vdate); len(entries) > 0 {
			fmt.Printf("WARNING: Skipping potential %d duplicate(s) for %s on %v (%v)\n", len(entries), assetId, vdate, entries[0].Type)
			continue
		}
		e := &kontoo.LedgerEntry{
			AssetID:        assetId,
			ValueDate:      vdate,
			Type:           kontoo.AssetPurchase,
			QuantityMicros: qty,
			ValueMicros:    val,
			PriceMicros:    val.Frac(kontoo.UnitValue, qty),
			CostMicros:     cost,
			Comment:        "Imported from xlsx logbook.",
		}
		assetIDs[assetId] = true
		data, _ := json.MarshalIndent(e, "", "  ")
		fmt.Println(string(data))
		err = store.Add(e)
		if err != nil {
			return fmt.Errorf("failed to import entry: %v", err)
		}
		nImported++
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("failed to save ledger: %v", err)
	}
	aIDs := make([]string, 0, len(assetIDs))
	for assetID := range assetIDs {
		aIDs = append(aIDs, assetID)
	}
	slices.Sort(aIDs)
	fmt.Println("Seen assets:", strings.Join(aIDs, "\n"))
	fmt.Println("Imported", nImported, "entries")
	return nil
}

func main() {
	if len(os.Args) == 1 {
		fmt.Printf("Please specify a valid command: [%s]\n",
			strings.Join([]string{"add", "serve", "import"}, ", "))
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "add":
		err = ProcessAdd(os.Args[2:])
	case "serve":
		err = ProcessServe(os.Args[2:])
	case "import":
		err = ProcessImport(os.Args[2:])
	default:
		err = fmt.Errorf("invalid command: %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
