package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/dnswlt/kontoo"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <SYMBOL>...\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}
	yf, err := kontoo.NewYFinanceCached(kontoo.CacheDir())
	if err != nil {
		log.Fatalf("Cannot create YFinance: %v", err)
	}
	loc, _ := time.LoadLocation("Europe/Zurich")
	var hist []*kontoo.DailyQuote
	if len(os.Args) > 2 {
		date := time.Now().In(loc)
		hist, err = yf.GetDailyQuotes(os.Args[1:], date)
	} else {
		end := time.Now().In(loc)
		start := end.AddDate(0, 0, -8)
		hist, err = yf.FetchPriceHistory(os.Args[1], start, end)
	}
	if err != nil {
		log.Fatalf("Cannot get history: %v", err)
	}
	s, _ := json.MarshalIndent(hist, "", "  ")
	fmt.Print(string(s))
}
