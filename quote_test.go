package kontoo

import (
	"encoding/json"
	"flag"
	"os"
	"path"
	"strings"
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

func TestFetchPriceHistory(t *testing.T) {
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
	hist, err := yf.FetchPriceHistory("MSFT", start, end)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	wantLen := 5
	if len(hist) != wantLen {
		t.Errorf("Wrong number of history items: want %d, got %d", wantLen, len(hist))
	}
	for _, h := range hist {
		if h.Currency != "USD" {
			t.Errorf("Wrong currency: want USD, got %s", h.Currency)
		}
		if h.Symbol != "MSFT" {
			t.Errorf("Wrong ticker: want MSFT, got %s", h.Symbol)
		}
		if h.ClosingPrice <= 0 {
			t.Errorf("Wrong closing price: got %v", h.ClosingPrice)
		}
		if h.Timestamp.Year() != 2024 || h.Timestamp.Month() != 2 {
			t.Errorf("Wrong date: got %v", h.Timestamp)
		}
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
	_, err = yf.FetchPriceHistory("DSNTEXST", start, end)
	if err != ErrTickerNotFound {
		t.Errorf("Expected ErrTickerNotFound, got %v", err)
	}
}

func TestGetDailyQuotesFuture(t *testing.T) {
	os.Setenv(cookieJarEnvVar, path.Join("./testdata", "yfcookiejar.json"))
	yf, err := NewYFinance()
	if err != nil {
		t.Fatalf("Failed to create YFinance: %v", err)
	}
	future := time.Now().Add(10 * 24 * time.Hour)
	_, err = yf.GetDailyQuotes([]string{"FOO"}, future)
	if err == nil || !strings.Contains(err.Error(), "in the past") {
		t.Errorf("Expected error for future date, got %q", err)
	}
}

var (
	tzNewYork, _ = time.LoadLocation("America/New_York")
	tzKolkata, _ = time.LoadLocation("Asia/Kolkata")
	tzBerlin, _  = time.LoadLocation("Europe/Berlin")
)

func TestPriceHistoryCacheAddAll_Timezones(t *testing.T) {
	tests := []struct {
		name       string
		quotes     []*DailyQuote
		start      time.Time
		end        time.Time
		wantDates  []string
		wantQuotes []int // index into quotes
	}{
		{
			name: "Single_quote",
			quotes: []*DailyQuote{
				{
					Symbol:    "V",
					Timestamp: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				},
			},
			start:      time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			end:        time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
			wantDates:  []string{"2024-01-15", "2024-01-16", "2024-01-17", "2024-01-18", "2024-01-19", "2024-01-20"},
			wantQuotes: []int{0, 0, 0, 0, 0, 0},
		},
		{
			name: "Timezone",
			quotes: []*DailyQuote{
				{
					Symbol:    "V",
					Timestamp: time.Date(2024, 1, 15, 9, 30, 0, 0, tzNewYork),
				},
				{
					Symbol:    "V",
					Timestamp: time.Date(2024, 1, 16, 9, 30, 0, 0, tzNewYork),
				},
				{
					Symbol:    "V",
					Timestamp: time.Date(2024, 1, 17, 9, 30, 0, 0, tzNewYork),
				},
			},
			start:      time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			end:        time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC),
			wantDates:  []string{"2024-01-15", "2024-01-16", "2024-01-17"},
			wantQuotes: []int{0, 1, 2},
		},
		{
			name: "First_last",
			quotes: []*DailyQuote{
				{
					Symbol:    "V",
					Timestamp: time.Date(2024, 1, 15, 9, 30, 0, 0, tzNewYork),
				},
				{
					Symbol:    "V",
					Timestamp: time.Date(2024, 1, 17, 9, 30, 0, 0, tzNewYork),
				},
			},
			start:      time.Date(2024, 1, 15, 12, 0, 0, 0, tzKolkata),
			end:        time.Date(2024, 1, 17, 12, 0, 0, 0, tzKolkata),
			wantDates:  []string{"2024-01-15", "2024-01-16", "2024-01-17"},
			wantQuotes: []int{0, 0, 1},
		},
		{
			name: "Ordering",
			quotes: []*DailyQuote{
				{
					Symbol:    "V",
					Timestamp: time.Date(2023, 12, 31, 9, 30, 0, 0, tzBerlin),
				},
				{
					Symbol:    "V",
					Timestamp: time.Date(2023, 12, 30, 9, 30, 0, 0, tzBerlin),
				},
			},
			start:      time.Date(2023, 12, 30, 0, 0, 0, 0, tzBerlin),
			end:        time.Date(2023, 12, 31, 0, 0, 0, 0, tzBerlin),
			wantDates:  []string{"2023-12-30", "2023-12-31"},
			wantQuotes: []int{1, 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.wantDates) != len(tc.wantQuotes) {
				t.Fatal("wantDates and wantQuotes must of the same length")
			}
			c := NewPriceHistoryCache()
			err := c.AddAll(tc.quotes, tc.start, tc.end)
			if err != nil {
				t.Fatalf("AddAll error: %v", err)
			}
			if len(c.entries) != len(tc.wantDates) {
				t.Errorf("Wrong #entries: got %d, want %d", len(c.entries), len(tc.wantDates))
			}
			for i := range tc.wantDates {
				key := quoteCacheKey{date: tc.wantDates[i], symbol: "V"}
				got, ok := c.entries[key]
				if !ok {
					t.Fatalf("No entry in cache for %v", key)
				}
				want := tc.quotes[tc.wantQuotes[i]]
				if got.quote != want {
					t.Errorf("Wrong entry for %q: got %v, want %v", tc.wantDates[i],
						got.quote.Timestamp, want.Timestamp)
				}
			}
		})
	}
}
