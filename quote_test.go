package kontoo

import (
	"encoding/json"
	"flag"
	"os"
	"path"
	"testing"
	"time"
)

var (
	integrationTest = flag.Bool("integration", false, "Set to true to activate integration tests")
)

func TestUnmarshalChart(t *testing.T) {
	data, err := os.ReadFile(path.Join("testdata", "chart.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cr YFChartResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		t.Fatalf("Cannot unmarshal: %v", err)
	}
	if len(cr.Chart.Result[0].Indicators.Quote[0].Close) == 0 {
		t.Error("no close data")
	}
	if len(cr.Chart.Result[0].Timestamps) == 0 {
		t.Error("no timestamps data")
	}
}

func TestUnmarshalChartError(t *testing.T) {
	data, err := os.ReadFile(path.Join("testdata", "chart_error.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cr YFChartResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		t.Fatalf("Cannot unmarshal: %v", err)
	}
	if cr.Chart.Error == nil {
		t.Error("no error")
	}
	if cr.Chart.Error.Code != "Not Found" {
		t.Errorf("wrong error code: %q", cr.Chart.Error.Code)
	}
}

func TestRequestChart(t *testing.T) {
	if !*integrationTest {
		t.Skip("Skipping integration test")
	}
	// Make sure to use an independent and non-existent cookie jar file.
	tempDir := t.TempDir()
	os.Setenv(cookieJarEnvVar, path.Join(tempDir, ".yfcookiejar"))

	yf, err := NewYFinance()
	if err != nil {
		t.Fatalf("Failed to create YFinance: %v", err)
	}
	start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 2, 8, 0, 0, 0, 0, time.UTC)
	hist, err := yf.GetPriceHistory("MSFT", start, end)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if hist.Currency != "USD" {
		t.Errorf("Wrong currency: want USD, got %s", hist.Currency)
	}
	if hist.Ticker != "MSFT" {
		t.Errorf("Wrong ticker: want MSFT, got %s", hist.Ticker)
	}
	wantLen := 5
	if len(hist.History) != wantLen {
		t.Errorf("Wrong number of history items: want %d, got %d", wantLen, len(hist.History))
	}
}

func TestRequestChartNotFound(t *testing.T) {
	if !*integrationTest {
		t.Skip("Skipping integration test")
	}
	// Make sure to use an independent and non-existent cookie jar file.
	tempDir := t.TempDir()
	os.Setenv(cookieJarEnvVar, path.Join(tempDir, ".yfcookiejar"))

	yf, err := NewYFinance()
	if err != nil {
		t.Fatalf("Failed to create YFinance: %v", err)
	}
	start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 2, 8, 0, 0, 0, 0, time.UTC)
	_, err = yf.GetPriceHistory("DSNTEXST", start, end)
	if err != ErrTickerNotFound {
		t.Errorf("Expected ErrTickerNotFound, got %v", err)
	}
}

// func TestDateTruncate(t *testing.T) {
// 	// Dates in cache entries are supposed to be YYYY-MM-DD dates only.
// 	// Make sure we're dealing with time-zone insanities properly.
// 	layout := time.RFC3339
// 	t1, _ := time.Parse(layout, "2024-07-01T13:00:00-07:00")
// 	t2, _ := time.Parse(layout, "2024-07-01T23:59:59-07:00")
// 	t1 = t1.Truncate(24 * time.Hour)
// 	t2 = t2.Truncate(24 * time.Hour)
// 	s1 := t1.Format(layout)
// 	if s1 != "2024-06-30T17:00:00-07:00" {
// 		t.Errorf("Unexpected time: %s", s1)
// 	}
// 	if s := t1.In(time.UTC).Format(layout); s != "FOO" {
// 		t.Errorf("Unexpected time: %s", s)
// 	}
// 	if !t1.Equal(t2) {
// 		t.Errorf("Times not equal: %v vs %v", t1, t2)
// 	}
// }

func TestHistoryCache(t *testing.T) {
	c := NewPriceHistoryCache(defaultCacheDir())
	hist := &PriceHistory{
		Ticker:   "FOO",
		Currency: "CHF",
		History: []PriceHistoryItem{
			{
				Date:         time.Date(2024, 1, 2, 17, 0, 0, 0, time.UTC),
				ClosingPrice: Micros(1_000_000),
			},
		},
	}
	if err := c.Add(hist); err != nil {
		t.Fatalf("failed to add cache entry: %v", err)
	}
}

func TestReadCsv(t *testing.T) {
	f := os.Getenv("HOME") + "/Downloads/depotuebersicht_1023663812_20240806-0842.csv"
	rows, err := ReadDepotExportCSVFile(f)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}
	if len(rows) != 10 {
		t.Errorf("wrong number of rows: %d", len(rows))
	}
}
