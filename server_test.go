package kontoo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestLoadTemplates(t *testing.T) {
	s := &Server{
		baseDir: ".",
	}
	if err := s.reloadTemplates(); err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}
}

// Function to count all elem elements in the HTML tree n.
func countElements(n *html.Node, elem string) int {
	count := 0
	if n.Type == html.ElementNode && n.Data == elem {
		count = 1
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		count += countElements(c, elem)
	}
	return count
}

func TestHandleLedger(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/kontoo/ledger", nil)
	w := httptest.NewRecorder()
	s, err := NewServer("localhost:8080", "./testdata/testledger.json", ".")
	if err != nil {
		t.Fatal("Cannot create server:", err)
	}
	s.handleLedger(w, r)
	if w.Code != http.StatusOK {
		t.Fatal("Wrong status:", w.Code)
	}
	h := w.Result().Header
	if ct := h.Get("Content-Type"); ct != "text/html" {
		t.Fatal("Wrong Content-Type:", ct)
	}
	doc, err := html.Parse(strings.NewReader(w.Body.String()))
	if err != nil {
		t.Fatal("Failed to parse HTML:", err)
	}
	// Want 3 <tr> elements: one for the <thead> and 2 for the ledger entries.
	if n := countElements(doc, "tr"); n != 3 {
		t.Errorf("Wrong number of <tr> elements in response: want 3, got %d", n)
	}
}

func TestHandlePositionsRedirect(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/kontoo/positions", nil)
	w := httptest.NewRecorder()
	s, err := NewServer("localhost:8080", "./testdata/testledger.json", ".")
	if err != nil {
		t.Fatal("Cannot create server:", err)
	}
	s.handlePositions(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatal("Wrong status:", w.Code)
	}
	h := w.Result().Header
	if ct := h.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Error("Wrong Content-Type:", ct)
	}
	wantLocation := fmt.Sprintf("/kontoo/positions?date=%s", today())
	if l := h.Get("Location"); l != wantLocation {
		t.Error("Wrong Location:", l, "want:", wantLocation)
	}
}

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper() // For better error reporting
	s, err := NewServer("localhost:8080", "./testdata/testledger.json", ".")
	if err != nil {
		t.Fatal("Cannot create server:", err)
	}
	srv := httptest.NewServer(s.createMux())
	return srv
}

func TestHandleAddEntriesPostWrongContentType(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	body := strings.NewReader(`{}`)
	resp, err := http.Post(srv.URL+"/kontoo/entries", "text/plain", body)
	if err != nil {
		t.Fatal("Post failed:", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Reading body failed:", err)
	}
	defer resp.Body.Close()
	if got := string(respBody); !strings.Contains(got, "application/json") {
		t.Error("Expected application/json in error message, got:", got)
	}
}

func TestHandleGetAllPaths(t *testing.T) {
	tests := []struct {
		path   string
		status int
	}{
		{"/", http.StatusOK},
		{"/kontoo/ledger", http.StatusOK},
		{"/kontoo/positions", http.StatusOK},
		{"/kontoo/positions/maturing", http.StatusOK},
		{"/kontoo/entries/new", http.StatusOK},
		{"/kontoo/assets/new", http.StatusOK},
		{"/kontoo/csv/upload", http.StatusOK},
		// Exclude /kontoo/quotes, as that would trigger Y! finance requests.
		{"/kontoo/positions/timeline", http.StatusMethodNotAllowed},
		{"/kontoo/entries", http.StatusMethodNotAllowed},
		{"/kontoo/assets", http.StatusMethodNotAllowed},
		{"/kontoo/csv", http.StatusMethodNotAllowed},
	}
	srv := setupTestServer(t)
	defer srv.Close()
	for _, tc := range tests {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatal("Get failed:", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Errorf("Expected status %d for path %q, got %d", tc.status, tc.path, resp.StatusCode)
		}
	}
}

func TestHandlePostJson(t *testing.T) {
	tests := []struct {
		path string
		data any
	}{
		// {"/kontoo/positions/timeline", },
		{"/kontoo/entries", &LedgerEntry{
			AssetID:        "NESN",
			QuantityMicros: 1 * UnitValue,
			PriceMicros:    95 * UnitValue,
		}},
		// {"/kontoo/assets", http.StatusMethodNotAllowed},
	}
	srv := setupTestServer(t)
	defer srv.Close()
	for _, tc := range tests {
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(tc.data)
		if err != nil {
			t.Fatal("Cannot marshal JSON:", err)
		}
		resp, err := http.Post(srv.URL+tc.path, "application/json", &buf)
		if err != nil {
			t.Fatal("Get failed:", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected OK status for path %q, got %d", tc.path, resp.StatusCode)
		}
	}
}
