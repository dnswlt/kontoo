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
