package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dnswlt/kontoo"
)

func main() {
	yf, err := kontoo.NewYFinance()
	if err != nil {
		log.Fatalf("Cannot create YFinance: %v", err)
	}
	end := time.Now()
	start := end.AddDate(0, 0, -8)
	hist, err := yf.GetPriceHistory(os.Args[1], start, end)
	if err != nil {
		log.Fatalf("Cannot get history: %v", err)
	}
	s, _ := json.MarshalIndent(hist, "", "  ")
	fmt.Print(string(s))
}
