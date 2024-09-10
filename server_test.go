package kontoo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if n := countElements(doc, "tr"); n < 3 {
		t.Errorf("Wrong number of <tr> elements in response: want at least 3, got %d", n)
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

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper() // For better error reporting
	// Copy testdata to temp directory and use that as the resource dir,
	// so that changes can be persisted, but only for the duration of the test.
	tempDir := t.TempDir()
	tempLedger := filepath.Join(tempDir, "testledger.json")
	copyFile("./testdata/testledger.json", tempLedger)
	s, err := NewServer("localhost:8080", tempLedger, ".")
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
	resp, err := http.Post(srv.URL+"/kontoo/entries/add", "text/plain", body)
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
		{"/kontoo/entries/add", http.StatusMethodNotAllowed},
		{"/kontoo/entries/delete", http.StatusMethodNotAllowed},
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
		{
			path: "/kontoo/entries/add",
			data: &LedgerEntry{
				Type:           AssetPurchase,
				AssetID:        "NESN",
				ValueDate:      DateVal(2024, 9, 1),
				QuantityMicros: 1 * UnitValue,
				PriceMicros:    95 * UnitValue,
			},
		},
		{
			path: "/kontoo/assets",
			data: &Asset{
				Type:         Stock,
				Name:         "Mercedes-Benz Group AG",
				TickerSymbol: "MBG.DE",
				Currency:     "EUR",
			},
		},
		{
			path: "/kontoo/positions/timeline",
			data: &PositionTimelineRequest{
				AssetIDs: []string{
					"NESN",
				},
				EndTimestamp: DateVal(2024, 12, 31).UnixMilli(),
				Period:       "1Y",
			},
		},
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
		// Every response should have these two fields.
		var r struct {
			Status StatusCode `json:"status"`
			Error  string     `json:"error"`
		}
		err = json.NewDecoder(resp.Body).Decode(&r)
		if err != nil {
			t.Fatalf("Cannot decode response: %v", err)
		}
		if r.Status != StatusOK {
			t.Errorf("Wrong status in response: want OK, got %v. Error: %q", r.Status, r.Error)
		}
	}
}

func TestHandlePostCsvUpload(t *testing.T) {
	// Uploads the testdata/positions.csv file as multipart/form-data
	// (like the UI does), and checks that it is processed succesfully.
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	filePart, err := w.CreateFormFile("file", "test.csv")
	if err != nil {
		t.Fatal("Cannot create form file:", err)
	}
	csvFile, err := os.Open("./testdata/positions.csv")
	if err != nil {
		t.Fatal("Could not open CSV file:", err)
	}
	defer csvFile.Close()
	_, err = io.Copy(filePart, csvFile)
	if err != nil {
		t.Fatal("Cannot copy to file part:", err)
	}
	w.Close()
	// Start the server.
	srv := setupTestServer(t)
	defer srv.Close()
	// Send the multipart/form-data request.
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/kontoo/csv", &body)
	if err != nil {
		t.Fatal("Failed to create request:", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("Failed to POST request:", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Failed to read response body:", err)
	}
	var r CsvUploadResponse
	err = json.NewDecoder(bytes.NewReader(data)).Decode(&r)
	if err != nil {
		t.Fatalf("Cannot decode response: %v. Response was: %q", err, string(data))
	}
	if r.Status != StatusOK {
		t.Errorf("Wrong status in response: want OK, got %v. Error: %q", r.Status, r.Error)
	}
	if r.NumEntries != 1 {
		t.Errorf("Wrong number of entries: want 1, got %d", r.NumEntries)
	}
	if r.InnerHTML == "" {
		t.Error("InnerHTML is missing")
	}
}
