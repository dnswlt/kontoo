package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/dnswlt/kontoo/internal/civil"
	"golang.org/x/net/html"
)

func TestSafeReturnURL(t *testing.T) {
	fallback := "/kontoo/ledger"
	for _, tc := range []struct{ raw, want string }{
		{"/kontoo/ledger?asset=account&page=2&sort=-value", "/kontoo/ledger?asset=account&page=2&sort=-value"},
		{"/kontoo/positions/maturing?date=2024-02-29", "/kontoo/positions/maturing?date=2024-02-29"},
		{"/kontoo/ledger?page=2&snippet=true", "/kontoo/ledger?page=2"},
		{"", fallback}, {"https://example.com/kontoo/ledger", fallback}, {"//example.com/kontoo/ledger", fallback},
		{"/\\example.com/kontoo/ledger", fallback}, {"/other-app", fallback},
		{"/kontoo/../elsewhere", fallback}, {"/kontoo/entries/new", fallback}, {"/kontoo/entries/edit/1", fallback},
		{"/kontoo/assets/new", fallback}, {"/kontoo/assets/edit/NESN", fallback},
	} {
		if got := safeReturnURL(tc.raw, fallback); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestEntryNavigationPreservesOrigin(t *testing.T) {
	s := ledgerViewTestServer(t)
	origin := "/kontoo/ledger?asset=account&page=2&sort=-value&date=2024-02-29"
	ctx := s.addCommonCtx(httptest.NewRequest(http.MethodGet, origin, nil), map[string]any{})
	nav := ctx["Nav"].(map[string]string)
	for _, key := range []string{"addEntry", "updateBalance"} {
		u, err := url.Parse(nav[key])
		if err != nil {
			t.Fatal(err)
		}
		if u.Query().Get("return_to") != origin || u.Query().Get("date") != "2024-02-29" {
			t.Fatalf("lost origin: %s", u)
		}
	}
	r := httptest.NewRequest(http.MethodGet, nav["addEntry"], nil)
	ctx = s.addCommonCtx(r, map[string]any{})
	if ctx["ReturnURL"] != origin {
		t.Fatal("entry page lost return destination")
	}
	reentry, _ := url.Parse(ctx["Nav"].(map[string]string)["addEntry"])
	if reentry.Query().Get("return_to") != origin {
		t.Fatal("entry navigation should not return to another entry page")
	}
}

// findElement returns the first <tag> element in n with attribute key=val.
func findElement(n *html.Node, tag, key, val string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag && htmlAttr(n, key) == val {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if e := findElement(c, tag, key, val); e != nil {
			return e
		}
	}
	return nil
}

func hasAttr(n *html.Node, key string) bool {
	return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool { return a.Key == key })
}

func htmlAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func TestEntryFormActionsAndTypeNames(t *testing.T) {
	s := ledgerViewTestServer(t)
	for _, update := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodGet, "/kontoo/entries/new?AssetID=account&prefill=balance&return_to=%2Fkontoo%2Fpositions%3Fdate%3D2024-02-29", nil)
		var b bytes.Buffer
		entry := s.store.FindEntryBySequenceNum(2)
		if !update {
			entry = nil
		}
		if err := s.renderEntryTemplate(&b, r, entry, civil.DateVal(2024, 2, 29)); err != nil {
			t.Fatal(err)
		}
		doc, err := html.Parse(&b)
		if err != nil {
			t.Fatal(err)
		}
		// Type stays a text input backed by the datalist, which is the fallback before JS runs.
		typeInput := findElement(doc, "input", "id", "Type")
		if typeInput == nil || htmlAttr(typeInput, "list") != "TypeList" {
			t.Fatal("Type should be a text input with list=TypeList")
		}
		want := "AccountBalance"
		if update {
			want = "AccountCredit"
		}
		if got := htmlAttr(typeInput, "value"); got != want {
			t.Errorf("update=%t: Type value: got %q, want %q", update, got, want)
		}
		typeList := findElement(doc, "datalist", "id", "TypeList")
		if typeList == nil || findElement(typeList, "option", "value", "AccountCredit") == nil ||
			findElement(typeList, "option", "value", "AccountBalance") == nil {
			t.Error("TypeList should offer entry types by their existing names")
		}
		const returnURL = "/kontoo/positions?date=2024-02-29"
		if form := findElement(doc, "form", "id", "entry-form"); form == nil || htmlAttr(form, "data-return-url") != returnURL {
			t.Errorf("update=%t: form should return to %s", update, returnURL)
		}
		if cancel := findElement(doc, "a", "class", "form-cancel"); cancel == nil || htmlAttr(cancel, "href") != returnURL {
			t.Errorf("update=%t: Cancel should link to %s", update, returnURL)
		}
		if findElement(doc, "button", "value", "save") == nil {
			t.Errorf("update=%t: missing Save button", update)
		}
		if hasNext := findElement(doc, "button", "value", "next") != nil; hasNext == update {
			t.Errorf("update=%t: Save & add another should be offered only for new entries", update)
		}
	}
}
