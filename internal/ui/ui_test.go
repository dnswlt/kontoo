package ui

import (
	"net/url"
	"testing"
	"time"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
)

func TestReportYearDates(t *testing.T) {
	tests := []struct {
		name string
		date civil.Date
		year int
		want string
	}{
		{"leap month end", civil.DateVal(2024, 2, 29), 2023, "2023-02-28"},
		{"month end into leap year", civil.DateVal(2023, 2, 28), 2024, "2024-02-29"},
		{"ordinary day", civil.DateVal(2024, 10, 10), 2023, "2023-10-10"},
		{"January month end", civil.DateVal(2024, 1, 31), 2023, "2023-01-31"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, _ := url.Parse("/kontoo/positions/maturing?date=2024-02-29&custom=keep")
			opts := YearOptions(*u, tc.date, civil.DateVal(2023, 1, 1), civil.DateVal(2024, 12, 31))
			for _, opt := range opts.Options {
				if opt.Value != tc.year {
					continue
				}
				got, err := url.Parse(opt.Data["URL"].(string))
				if err != nil {
					t.Fatal(err)
				}
				if got.Query().Get("date") != tc.want || got.Query().Get("custom") != "keep" || got.Path != u.Path {
					t.Fatalf("year navigation lost date or report context: %s", got)
				}
				return
			}
			t.Fatalf("year %d missing", tc.year)
		})
	}
}

func TestReportYearsWithoutEntries(t *testing.T) {
	u, _ := url.Parse("/kontoo/positions?date=2026-10-10")
	date := civil.DateVal(2026, 10, 10)
	opts := YearOptions(*u, date, civil.Date{}, civil.Date{})
	if len(opts.Options) != 1 || opts.Options[0].Value != 2026 {
		t.Fatalf("empty ledger should offer the report year: %+v", opts.Options)
	}
	opts = YearOptions(*u, date, civil.DateVal(2023, 1, 1), civil.DateVal(2024, 12, 31))
	if len(opts.Options) != 4 || opts.Options[0].Value != 2026 {
		t.Fatalf("report year must remain available beyond the last ledger entry: %+v", opts.Options)
	}
}

func TestReportMonthDates(t *testing.T) {
	for _, year := range []int{2023, 2024, 2026} {
		date := civil.DateVal(year, 10, 10)
		u, _ := url.Parse("/kontoo/positions/equity?custom=keep")
		opts := MonthOptions(*u, date)
		if len(opts.Options) != 12 {
			t.Fatalf("want all twelve months, got %d", len(opts.Options))
		}
		for _, opt := range opts.Options {
			got, err := url.Parse(opt.Data["URL"].(string))
			if err != nil {
				t.Fatal(err)
			}
			d, err := civil.ParseDate(got.Query().Get("date"))
			if err != nil {
				t.Fatal(err)
			}
			if d.Year() != year || int(d.Month()) != opt.Value || d.AddDays(1).Day() != 1 {
				t.Errorf("month option must navigate to its month end: %s", got)
			}
			if got.Path != u.Path || got.Query().Get("custom") != "keep" {
				t.Errorf("month navigation lost report context: %s", got)
			}
		}
	}
}

func TestJoinAny(t *testing.T) {
	tests := []struct {
		input   any
		sep     string
		want    string
		wantErr bool
	}{
		{[]string{"foo", "bar"}, " ", "foo bar", false},
		{[]string{}, ":", "", false},
		{[]int{1, 2, 3}, ", ", "1, 2, 3", false},
		{[]bool{true, false}, ":", "true:false", false},
		{[]struct{}{}, " ", "", false},
		{[]ledger.EntryType{ledger.AssetPurchase, ledger.AssetSale}, " ", "AssetPurchase AssetSale", false},
		{[]ledger.EntryType{ledger.AssetPurchase, ledger.AssetSale}, " ", "AssetPurchase AssetSale", false},
		{[]civil.Date{civil.DateVal(2000, 1, 1)}, " ", "2000-01-01", false},
		{1.3, "", "", true},
		{time.Now(), "", "", true},
	}
	for _, tc := range tests {
		got, err := joinAny(tc.input, tc.sep)
		if err != nil && !tc.wantErr {
			t.Fatal(err)
		}
		if err == nil && tc.wantErr {
			t.Fatal("Wanted error, got none")
		}
		if got != tc.want {
			t.Errorf("Want: %q, got: %q", tc.want, got)
		}
	}
}

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		end    civil.Date
		period string
		want   civil.Date
	}{
		{civil.DateVal(2024, 1, 1), "1M", civil.DateVal(2023, 12, 1)},
		{civil.DateVal(2024, 2, 29), "1Y", civil.DateVal(2023, 3, 1)},
		{civil.DateVal(1999, 12, 1), "5D", civil.DateVal(1999, 11, 26)},
		{civil.DateVal(2024, 1, 1), "Max", civil.Date{}},
		{civil.DateVal(2023, 7, 31), "YTD", civil.DateVal(2023, 1, 1)},
		{civil.DateVal(2023, 1, 1), "YTD", civil.DateVal(2023, 1, 1)},
	}
	for _, tc := range tests {
		got, err := ParsePeriod(tc.end, tc.period)
		if err != nil {
			t.Fatal("Cannot parse period:", err)
		}
		if !got.Equal(tc.want) {
			t.Errorf("Want: %v, got: %v", tc.want, got)
		}
	}
}
