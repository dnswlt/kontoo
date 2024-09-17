package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dnswlt/kontoo"
)

type dateFlag struct {
	Time  time.Time
	Valid bool
}

func (tf *dateFlag) Set(value string) error {
	parsedTime, err := time.Parse("2006-01-02", value)
	if err != nil {
		return fmt.Errorf("invalid date format, use YYYY-MM-DD")
	}
	tf.Time = parsedTime
	tf.Valid = true
	return nil
}

func (tf *dateFlag) String() string {
	if !tf.Valid {
		return ""
	}
	return tf.Time.Format("2006-01-02")
}

func main() {
	var (
		startDate     dateFlag
		endDate       dateFlag
		symbolsStr    = flag.String("symbols", "", "Comma-separated list of symbols to fetch data for")
		baseCurrency  = flag.String("base-currency", "EUR", "Base currency to use for exchange rates")
		currenciesStr = flag.String("currencies", "", "Comma-separated list of currencies to fetch data for")
		ledger        = flag.String("ledger", "", "Path to a ledger JSON file into which to import the data")
		quarterly     = flag.Bool("quarterly", true, "Only import quarterly prices into ledger")
	)
	flag.Var(&startDate, "start", "Start date of the period to fetch")
	flag.Var(&endDate, "end", "End date of the period to fetch")
	flag.Parse()

	if !startDate.Valid || !endDate.Valid {
		fmt.Fprintln(os.Stderr, "No valid start and/or end date given")
		os.Exit(1)
	}
	yf, err := kontoo.NewYFinance()
	if err != nil {
		log.Fatalf("Cannot create YFinance: %v", err)
	}
	var store *kontoo.Store
	if *ledger != "" {
		if s, err := kontoo.LoadStore(*ledger); err != nil {
			log.Fatal("Cannot load store:", err)
		} else {
			store = s
		}
	}
	if *symbolsStr != "" {
		symbols := strings.Split(*symbolsStr, ",")
		for _, sym := range symbols {
			hist, err := yf.FetchPriceHistory(sym, startDate.Time, endDate.Time)
			if err != nil {
				log.Fatalf("Cannot get history: %v", err)
			}
			for i := 0; i < len(hist); i++ {
				isLastOfMonth := i == len(hist)-1 || hist[i].Timestamp.Month() != hist[i+1].Timestamp.Month()
				isQuarter := isLastOfMonth && hist[i].Timestamp.Month()%3 == 0
				if isLastOfMonth && (!*quarterly || isQuarter) {
					// Print only last entries of each month.
					h := hist[i]
					fmt.Printf("%s %s %v\n", h.Timestamp.Format("2006-01-02"), h.Symbol, h.ClosingPrice.Format(".3"))
					if store != nil {
						err := store.Add(&kontoo.LedgerEntry{
							Type:        kontoo.AssetPrice,
							ValueDate:   kontoo.ToDate(h.Timestamp),
							AssetRef:    h.Symbol,
							PriceMicros: h.ClosingPrice,
						})
						if err != nil {
							log.Fatalf("Error adding to ledger: %v", err)
						}
					}
				}
			}
		}
	}
	if *currenciesStr != "" {
		currencies := strings.Split(*currenciesStr, ",")
		for _, ccy := range currencies {
			hist, err := yf.FetchPriceHistory(fmt.Sprintf("%s%s=X", *baseCurrency, ccy), startDate.Time, endDate.Time)
			if err != nil {
				log.Fatalf("Cannot get history: %v", err)
			}
			for i := 0; i < len(hist); i++ {
				isLastOfMonth := i == len(hist)-1 || hist[i].Timestamp.Month() != hist[i+1].Timestamp.Month()
				isQuarter := isLastOfMonth && hist[i].Timestamp.Month()%3 == 0
				if isLastOfMonth && (!*quarterly || isQuarter) {
					// Print only last entries of each month.
					h := hist[i]
					fmt.Printf("%s %s %s/%s %v\n", h.Timestamp.Format("2006-01-02"), h.Symbol, *baseCurrency, h.Currency, h.ClosingPrice.Format(".3"))
					if store != nil {
						err := store.Add(&kontoo.LedgerEntry{
							Type:          kontoo.ExchangeRate,
							ValueDate:     kontoo.ToDate(h.Timestamp),
							PriceMicros:   h.ClosingPrice,
							Currency:      kontoo.Currency(*baseCurrency),
							QuoteCurrency: h.Currency,
						})
						if err != nil {
							log.Fatalf("Error adding to ledger: %v", err)
						}
					}
				}
			}
		}
	}
	if store != nil {
		store.Save()
	}
}
