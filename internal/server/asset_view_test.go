package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"golang.org/x/net/html"
)

// The form must only offer fields that validation accepts for the asset type.
func TestAssetFormFieldsPassValidation(t *testing.T) {
	for _, typ := range ledger.AssetTypeValues()[1:] {
		store, err := ledger.NewStore(&ledger.Ledger{Header: &ledger.LedgerHeader{BaseCurrency: "CHF"}}, "")
		if err != nil {
			t.Fatal(err)
		}
		a := &ledger.Asset{Type: typ, Name: "Test", Currency: "CHF", CustomID: "test"}
		for _, f := range assetFormFields(typ) {
			switch f {
			case "ISIN":
				a.ISIN = "CH0038863350"
			case "WKN":
				a.WKN = "A0Q4DC"
			case "TickerSymbol":
				a.TickerSymbol = "NESN"
			case "YFSymbol":
				a.QuoteServiceSymbols = map[string]string{"YF": "NESN.SW"}
			case "IBAN":
				a.IBAN = "CH9300762011623852957"
			case "AccountNumber":
				a.AccountNumber = "123"
			case "IssueDate":
				d := civil.DateVal(2024, 1, 1)
				a.IssueDate = &d
			case "MaturityDate":
				d := civil.DateVal(2030, 1, 1)
				a.MaturityDate = &d
			case "Interest":
				a.InterestMicros = 25_000
			case "InterestPayment":
				a.InterestPayment = ledger.AnnualPayment
			default:
				t.Fatalf("%v: unknown form field %q", typ, f)
			}
		}
		if err := store.AddAsset(a); err != nil {
			t.Errorf("%v: fields %v fail validation: %v", typ, assetFormFields(typ), err)
		}
	}
}

func renderAssetForm(t *testing.T, s *Server, target string, asset *ledger.Asset) *html.Node {
	t.Helper()
	var b bytes.Buffer
	if err := s.renderAssetTemplate(&b, httptest.NewRequest(http.MethodGet, target, nil), asset); err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(&b)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestAssetFormNew(t *testing.T) {
	s := ledgerViewTestServer(t)
	doc := renderAssetForm(t, s, "/kontoo/assets/new?return_to=%2Fkontoo%2Fpositions", nil)
	if findElement(doc, "input", "name", "AssetId") != nil {
		t.Error("new asset form should not carry an AssetId")
	}
	if c := findElement(doc, "input", "id", "Currency"); c == nil || htmlAttr(c, "value") != "CHF" {
		t.Error("new assets should default to the base currency")
	}
	if form := findElement(doc, "form", "id", "asset-form"); form == nil || htmlAttr(form, "data-return-url") != "/kontoo/positions" {
		t.Error("form should return to its origin")
	}
	if findElement(doc, "button", "value", "next") == nil {
		t.Error("new asset form should offer Save & add another")
	}
	bond := findElement(doc, "option", "value", "GovernmentBond")
	if bond == nil || htmlAttr(bond, "data-fields") != "ISIN WKN TickerSymbol YFSymbol IssueDate MaturityDate Interest InterestPayment" {
		t.Error("asset type options should list their type-specific fields")
	}
}

func TestAssetFormEditWithEntries(t *testing.T) {
	s := ledgerViewTestServer(t)
	asset, _ := s.Store().Asset("account")
	doc := renderAssetForm(t, s, "/kontoo/assets/edit/account", asset)
	if id := findElement(doc, "input", "name", "AssetId"); id == nil || htmlAttr(id, "value") != "account" {
		t.Error("edit form should carry the asset ID")
	}
	c := findElement(doc, "input", "id", "Currency")
	if c == nil {
		t.Fatal("missing Currency input")
	}
	if !hasAttr(c, "readonly") {
		t.Error("currency of an asset with entries should be read-only")
	}
	// The checking account's category is "Cash equivalents".
	if !hasAttr(findElement(doc, "option", "value", "Stock"), "disabled") ||
		hasAttr(findElement(doc, "option", "value", "SavingsAccount"), "disabled") {
		t.Error("only types of the same category should be enabled for an asset with entries")
	}
	if findElement(doc, "button", "value", "next") != nil {
		t.Error("edit form should not offer Save & add another")
	}
}

func TestAssetUpdateKeepsExchangeTimezone(t *testing.T) {
	store, err := ledger.NewStore(&ledger.Ledger{
		Header: &ledger.LedgerHeader{BaseCurrency: "CHF"},
		Assets: []*ledger.Asset{{Type: ledger.Stock, Name: "Nestle", TickerSymbol: "NESN", Currency: "CHF",
			QuoteServiceSymbols: map[string]string{"YF": "NESN.SW"}, ExchangeTimezone: "Europe/Zurich"}},
	}, filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: store}
	post := func(yf string) *ledger.Asset {
		body, _ := json.Marshal(UpsertAssetRequest{AssetID: "NESN", Asset: &ledger.Asset{Type: ledger.Stock,
			Name: "Nestle SA", TickerSymbol: "NESN", Currency: "CHF", QuoteServiceSymbols: map[string]string{"YF": yf}}})
		w := httptest.NewRecorder()
		s.handleAssetsPost(w, httptest.NewRequest(http.MethodPost, "/kontoo/assets", bytes.NewReader(body)))
		if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"OK"`)) {
			t.Fatalf("update failed: %d %s", w.Code, w.Body)
		}
		a, _ := store.Asset("NESN")
		return a
	}
	if a := post("NESN.SW"); a.ExchangeTimezone != "Europe/Zurich" || a.Name != "Nestle SA" {
		t.Errorf("unchanged symbol should keep timezone: %+v", a)
	}
	if a := post("NESN.VX"); a.ExchangeTimezone != "" {
		t.Errorf("changed symbol should drop the old timezone, got %q", a.ExchangeTimezone)
	}
}
