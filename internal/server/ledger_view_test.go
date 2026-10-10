package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/units"
)

func ledgerViewTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := ledger.NewStore(&ledger.Ledger{
		Header: &ledger.LedgerHeader{BaseCurrency: "CHF"},
		Assets: []*ledger.Asset{{CustomID: "account", Name: "Alpha Bank", Type: ledger.CheckingAccount, Currency: "CHF"}, {CustomID: "stock", Name: "Beta stock", Type: ledger.Stock, Currency: "USD"}},
		Entries: []*ledger.LedgerEntry{
			{SequenceNum: 1, AssetID: "account", Currency: "CHF", Type: ledger.AccountBalance, ValueDate: civil.DateVal(2024, 2, 27), ValueMicros: 100 * units.One},
			{SequenceNum: 2, AssetID: "account", Currency: "CHF", Type: ledger.AccountCredit, ValueDate: civil.DateVal(2024, 2, 28), ValueMicros: 25 * units.One, Comment: "note: fee=0"},
			{SequenceNum: 3, AssetID: "stock", Currency: "USD", Type: ledger.AssetPurchase, ValueDate: civil.DateVal(2024, 2, 29), QuantityMicros: units.One, PriceMicros: 10 * units.One},
			{SequenceNum: 4, AssetID: "stock", Currency: "USD", Type: ledger.AssetPrice, ValueDate: civil.DateVal(2024, 3, 1), PriceMicros: 20 * units.One},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: store, baseDir: "../../resources"}
	if err := s.reloadTemplates(); err != nil {
		t.Fatal(err)
	}
	return s
}

func paginatedLedgerTestServer(t *testing.T, count int) *Server {
	t.Helper()
	s := ledgerViewTestServer(t)
	l := &ledger.Ledger{
		Header: &ledger.LedgerHeader{BaseCurrency: "CHF"},
		Assets: []*ledger.Asset{{CustomID: "account", Name: "Alpha Bank", Type: ledger.CheckingAccount, Currency: "CHF"}},
	}
	for n := 1; n <= count; n++ {
		comment := "odd"
		if n%2 == 0 {
			comment = "even"
		}
		l.Entries = append(l.Entries, &ledger.LedgerEntry{
			SequenceNum: int64(n), AssetID: "account", Currency: "CHF", Type: ledger.AccountCredit,
			ValueDate: civil.DateVal(2024, 2, 28), ValueMicros: units.One, Comment: comment,
		})
	}
	var err error
	s.store, err = ledger.NewStore(l, "")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLedgerPagination(t *testing.T) {
	s := paginatedLedgerTestServer(t, 250)
	for _, tc := range []struct {
		params                                             string
		page, pages, total, first, last, firstSeq, lastSeq int
	}{
		{"", 1, 3, 250, 1, 100, 250, 151},
		{"page=2", 2, 3, 250, 101, 200, 150, 51},
		{"page=3", 3, 3, 250, 201, 250, 50, 1},
		{"page=999999999", 3, 3, 250, 201, 250, 50, 1},
		{"page=2&sort=num", 2, 3, 250, 101, 200, 101, 200},
		{"page=2&sort=num&search=even", 2, 2, 125, 101, 125, 202, 250},
	} {
		t.Run(tc.params, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/kontoo/ledger?"+tc.params, nil)
			ctx, err := s.ledgerViewContext(r)
			if err != nil {
				t.Fatal(err)
			}
			v := ctx["Filters"].(*ledgerView)
			rows := ctx["TableRows"].([]*ledger.LedgerEntryRow)
			if v.Page != tc.page || v.PageCount != tc.pages || v.EntryCount != tc.total || v.FirstEntry != tc.first || v.LastEntry != tc.last || len(rows) != tc.last-tc.first+1 {
				t.Fatalf("incorrect page bounds: %+v, rows: %d", v, len(rows))
			}
			if rows[0].SequenceNum() != int64(tc.firstSeq) || rows[len(rows)-1].SequenceNum() != int64(tc.lastSeq) {
				t.Fatalf("filtering and sorting must precede pagination: first %d, last %d", rows[0].SequenceNum(), rows[len(rows)-1].SequenceNum())
			}
			if rows[0].TotalMarketValue() != units.Micros(tc.firstSeq)*units.One {
				t.Fatal("pagination must preserve running balances from all preceding entries")
			}
			w := httptest.NewRecorder()
			s.handleLedger(w, r)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "aria-label=\"Ledger pages\"") {
				t.Fatalf("page navigation did not render: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestLedgerPaginationBoundaries(t *testing.T) {
	for _, count := range []int{0, 1, 100, 101} {
		s := paginatedLedgerTestServer(t, count)
		ctx, err := s.ledgerViewContext(httptest.NewRequest(http.MethodGet, "/kontoo/ledger?page=3", nil))
		if err != nil {
			t.Fatal(err)
		}
		v := ctx["Filters"].(*ledgerView)
		rows := ctx["TableRows"].([]*ledger.LedgerEntryRow)
		wantPage, wantRows := 1, count
		if count == 101 {
			wantPage, wantRows = 2, 1
		}
		if v.Page != wantPage || v.PageCount != wantPage || len(rows) != wantRows || v.EntryCount != count {
			t.Fatalf("count %d: incorrect boundary: %+v, rows %d", count, v, len(rows))
		}
		if count == 0 && (v.FirstEntry != 0 || v.LastEntry != 0) {
			t.Fatal("empty results should have no range")
		}
	}
	s := paginatedLedgerTestServer(t, 101)
	for _, page := range []string{"0", "-1", "abc", "1.5", "999999999999999999999999999999"} {
		w := httptest.NewRecorder()
		s.handleLedger(w, httptest.NewRequest(http.MethodGet, "/kontoo/ledger?page="+page, nil))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "page must be a positive whole number") {
			t.Fatalf("invalid page %s: status %d", page, w.Code)
		}
	}
}

func TestLedgerPageLinks(t *testing.T) {
	s := paginatedLedgerTestServer(t, 450)
	params := "asset=account&search=even&from=2024-02-01&until=2024-03-01&date=2024-02-29&include_market=0&sort=num&page=2"
	ctx, err := s.ledgerViewContext(httptest.NewRequest(http.MethodGet, "/kontoo/ledger?"+params, nil))
	if err != nil {
		t.Fatal(err)
	}
	v := ctx["Filters"].(*ledgerView)
	for link, page := range map[string]string{v.FirstPageURL: "", v.PreviousPageURL: "", v.NextPageURL: "3", v.LastPageURL: "3"} {
		u, err := url.Parse(link)
		if err != nil || link == "" {
			t.Fatalf("invalid page URL %q: %v", link, err)
		}
		q := u.Query()
		if q.Get("page") != page || q.Get("asset") != "account" || q.Get("search") != "even" || q.Get("from") != "2024-02-01" || q.Get("until") != "2024-03-01" || q.Get("date") != "2024-02-29" || q.Get("include_market") != "0" || q.Get("sort") != "num" {
			t.Fatalf("page navigation lost context: %s", link)
		}
	}
	for _, link := range []string{v.ClearURL, v.SortHeaders["value"].URL, v.Chips[0].URL, v.Chips[1].URL, v.AssetClearURL, v.DateClearURL} {
		u, _ := url.Parse(link)
		if u.Query().Has("page") {
			t.Fatalf("changing filters or sorting must reset page: %s", link)
		}
	}
}

func TestLedgerViewFilters(t *testing.T) {
	s := ledgerViewTestServer(t)
	tests := []struct {
		params          string
		count           int
		market, invalid bool
	}{
		{"", 4, true, false},
		{"asset=stock", 2, true, false},
		{"asset=account&asset=stock", 4, true, false},
		{"search=note%3A+fee%3D0", 1, true, false},
		{"include_market=0", 3, false, false},
		{"include_market=1", 4, true, false},
		{"include_market=1&include_market=0", 4, true, false},
		{"include_market=0&include_market=1", 4, true, false},
		{"from=2024-02-28&until=2024-02-29", 2, true, false},
		{"from=2024-03-01&until=2024-02-01", 0, true, true},
		{"asset=missing", 0, true, true},
		{"sort=invalid", 0, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.params, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/kontoo/ledger?"+tc.params, nil)
			ctx, err := s.ledgerViewContext(r)
			v := ctx["Filters"].(*ledgerView)
			if (err != nil) != tc.invalid || v.EntryCount != tc.count || v.IncludeMarket != tc.market {
				t.Fatalf("unexpected filter result: %+v, error: %v", v, err)
			}
			w := httptest.NewRecorder()
			s.handleLedger(w, r)
			wantStatus := http.StatusOK
			if tc.invalid {
				wantStatus = http.StatusBadRequest
			}
			if w.Code != wantStatus || !strings.Contains(w.Body.String(), "<h1>Ledger</h1>") {
				t.Fatalf("expected usable ledger page, got %d: %s", w.Code, w.Body)
			}
		})
	}
}

func TestLedgerFiltersPreserveContext(t *testing.T) {
	s := ledgerViewTestServer(t)
	r := httptest.NewRequest(http.MethodGet, "/kontoo/ledger?asset=account&asset=stock&search=note%3A&date=2024-02-29&sort=-value", nil)
	ctx, err := s.ledgerViewContext(r)
	if err != nil {
		t.Fatal(err)
	}
	v := ctx["Filters"].(*ledgerView)
	for _, chip := range v.Chips {
		if chip.Label != "Asset: Alpha Bank" {
			continue
		}
		u, _ := url.Parse(chip.URL)
		if u.Query().Get("asset") != "stock" || u.Query().Get("search") != "note:" || u.Query().Get("date") != "2024-02-29" || u.Query().Get("sort") != "-value" {
			t.Fatalf("removing one chip lost context: %s", u)
		}
	}
	for _, link := range []string{v.AssetClearURL, v.DateClearURL} {
		u, _ := url.Parse(link)
		if u.Query().Get("search") != "note:" || u.Query().Get("date") != "2024-02-29" || u.Query().Get("sort") != "-value" {
			t.Fatalf("clearing one filter lost context: %s", u)
		}
	}
	if u, _ := url.Parse(v.AssetClearURL); u.Query().Has("asset") {
		t.Fatalf("clearing the asset filter kept assets: %s", u)
	}
	u, _ := url.Parse(v.SortHeaders["value"].URL)
	if u.Query().Get("sort") != "value" || len(u.Query()["asset"]) != 2 || u.Query().Get("date") != "2024-02-29" {
		t.Fatalf("sort lost context: %s", u)
	}
	r = httptest.NewRequest(http.MethodGet, "/kontoo/ledger?asset=account&from=2024-02-28", nil)
	ctx, err = s.ledgerViewContext(r)
	if err != nil {
		t.Fatal(err)
	}
	rows := ctx["TableRows"].([]*ledger.LedgerEntryRow)
	if len(rows) != 1 || rows[0].TotalMarketValue() != 125*units.One {
		t.Fatal("filtering must preserve accumulated position values from the full ledger")
	}
}
