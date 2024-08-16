package kontoo

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

type DailyExchangeRate struct {
	BaseCurrency  Currency
	QuoteCurrency Currency
	Timestamp     time.Time // Timestamp for the ClosingPrice as received from the quote service.
	ClosingPrice  Micros    // Expressed as a multiple of the QuoteCurrency: 1.30 means for 1 BaseCurrency you get 1.30 QuoteCurrency.
}

type DailyQuote struct {
	Symbol       string
	Currency     Currency
	Timestamp    time.Time // Timestamp for the ClosingPrice as received from the quote service.
	ClosingPrice Micros
}

type YFinance struct {
	client    *http.Client
	cookieJar CookieJar
	cache     *PriceHistoryCache
}

type SimpleCookie struct {
	Name    string
	Value   string
	Expires time.Time
}
type CookieJar struct {
	Crumb   string
	Cookies []SimpleCookie
}

// Y! Finance API
type YFChartResponse struct {
	Chart *YFChart `json:"chart"`
}
type YFError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}
type YFChart struct {
	Result []*YFChartResult `json:"result"`
	Error  *YFError         `json:"error"`
}
type YFChartResult struct {
	Meta       *YFMeta       `json:"meta"`
	Indicators *YFIndicators `json:"indicators"`
	Timestamps []float64     `json:"timestamp"`
}
type YFMeta struct {
	ExchangeTimezoneName string `json:"exchangeTimezoneName"`
	Currency             string `json:"currency"`
	Symbol               string `json:"symbol"`
}
type YFIndicators struct {
	Quote []*YFQuote `json:"quote"`
}
type YFQuote struct {
	Close []float64 `json:"close"`
}

const (
	userAgent       = `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15`
	cookieJarEnvVar = "YFCOOKIEJAR"
	cacheDirEnvVar  = "KONTOO_CACHE"
)

func (j *CookieJar) AddCookies(req *http.Request) {
	for _, c := range j.Cookies {
		req.AddCookie(&http.Cookie{
			Name:  c.Name,
			Value: c.Value,
		})
	}
}

func NewYFinance() (*YFinance, error) {
	yf := &YFinance{
		client: &http.Client{},
		cache:  NewPriceHistoryCache(),
	}
	if err := yf.LoadCookieJar(); err != nil {
		if os.IsNotExist(err) || errors.Is(err, ErrCookiesExpired) {
			if err := yf.RefreshCookieJar(); err != nil {
				return nil, err
			}
		}
	}
	return yf, nil
}

var (
	ErrCookiesExpired = errors.New("cookies in cookie jar expired")
	ErrTickerNotFound = errors.New("ticker symbol not found")
	ErrNotCached      = errors.New("requested entry not found in cache")
)

func (yf *YFinance) cookieJarFile() string {
	if f := os.Getenv(cookieJarEnvVar); f != "" {
		return f
	}
	filename := ".yfcookiejar"
	if home, err := os.UserHomeDir(); err == nil {
		return path.Join(home, filename)
	}
	return path.Join(".", filename)
}

func (yf *YFinance) LoadCookieJar() error {
	data, err := os.ReadFile(yf.cookieJarFile())
	if err != nil {
		return err
	}
	var jar CookieJar
	if err := json.Unmarshal(data, &jar); err != nil {
		log.Printf("Cannot unmarshal cookie jar: %v", err)
		return fmt.Errorf("cannot unmarshal cookie jar: %w", err)
	}
	minExpires := time.Now().Add(1 * time.Hour)
	for _, c := range jar.Cookies {
		if c.Expires.Before(minExpires) {
			log.Printf("Cookies expired at %v", c.Expires)
			return ErrCookiesExpired
		}
	}
	yf.cookieJar = jar
	return nil
}

func (yf *YFinance) RefreshCookieJar() error {
	cookies, err := yf.getCookies()
	if err != nil {
		return err
	}
	var cookieJar CookieJar
	cookieJar.Cookies = make([]SimpleCookie, len(cookies))
	for i, c := range cookies {
		var expires = c.Expires
		if expires.IsZero() && c.MaxAge > 0 {
			expires = time.Now().Add(time.Duration(c.MaxAge) * time.Second)
		}
		cookieJar.Cookies[i] = SimpleCookie{
			Name:    c.Name,
			Value:   c.Value,
			Expires: expires,
		}
	}
	yf.cookieJar = cookieJar
	crumb, err := yf.getCrumb()
	if err != nil {
		return err
	}
	cookieJar.Crumb = crumb
	// Try to save the jar to disk.
	data, err := json.Marshal(cookieJar)
	if err != nil {
		log.Fatalf("Cannot marshal JSON: %v", err)
	}
	jarFile := yf.cookieJarFile()
	if err := os.WriteFile(jarFile, data, 0644); err != nil {
		log.Printf("Cannot write cookie jar to %q: %v", jarFile, err)
		// Don't treat this as an error: if we're on a diskless machine,
		// we'll just use the in-memory crumb.
	}
	return nil
}

func (yf *YFinance) getCookies() ([]*http.Cookie, error) {
	url := "https://fc.yahoo.com"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create cookie request: %w", err)
	}
	req.Header.Add("User-Agent", userAgent)
	log.Printf("Fetching cookies from %s", url)
	resp, err := yf.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cookie request failed: %w", err)
	}
	log.Printf("Cookie request to %s returned status %s", url, resp.Status)
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read cookie response body: %w", err)
	}
	return resp.Cookies(), nil
}

func (yf *YFinance) getCrumb() (string, error) {
	url := "https://query1.finance.yahoo.com/v1/test/getcrumb"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("cannot create crumb request: %w", err)
	}
	req.Header.Add("User-Agent", userAgent)
	yf.cookieJar.AddCookies(req)
	log.Printf("Fetching crumb from %s", url)
	resp, err := yf.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("crumb request failed: %w", err)
	}
	log.Printf("Crumb request to %s returned status %s", url, resp.Status)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read crumb body: %w", err)
	}
	return string(body), nil
}

// Get closing exchange rates of multiple currencies for a single day.
func (yf *YFinance) GetDailyExchangeRates(baseCurrency Currency, quoteCurrencies []Currency, date time.Time) ([]*DailyExchangeRate, error) {
	// For Y! Finance, exchange rates are just quotes, with a special ticker symbol encoding.
	// We can use GetDailyQuotes to obtain the rates, and just have to transform the output data structure.
	symbols := make([]string, len(quoteCurrencies))
	for i, q := range quoteCurrencies {
		symbols[i] = fmt.Sprintf("%s%s=X", baseCurrency, q)
	}
	quotes, err := yf.GetDailyQuotes(symbols, date)
	if err != nil {
		return nil, err
	}
	res := make([]*DailyExchangeRate, len(quotes))
	for i, q := range quotes {
		res[i] = &DailyExchangeRate{
			BaseCurrency:  baseCurrency,
			QuoteCurrency: q.Currency,
			Timestamp:     q.Timestamp,
			ClosingPrice:  q.ClosingPrice,
		}
	}
	return res, nil
}

// Get closing prices of multiple stocks for a single day.
func (yf *YFinance) GetDailyQuotes(symbols []string, date time.Time) ([]*DailyQuote, error) {
	if date.After(time.Now()) {
		return nil, fmt.Errorf("date must be in the past, was %v", date)
	}
	var result []*DailyQuote
	var uncached []string
	if yf.cache != nil {
		for _, sym := range symbols {
			cached, err := yf.cache.Get(sym, date)
			if err != nil {
				if errors.Is(err, ErrNotCached) {
					uncached = append(uncached, sym)
					continue
				}
				return nil, fmt.Errorf("failed to read from cache: %w", err)
			}
			result = append(result, cached)
		}
	} else {
		uncached = symbols
	}
	startDate := date.AddDate(0, 0, -8)
	for _, sym := range uncached {
		hist, err := yf.FetchPriceHistory(sym, startDate, date)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch price history for %s: %w", sym, err)
		}
		if len(hist) == 0 {
			return nil, fmt.Errorf("no results when fetching price history for %s", sym)
		}
		err = yf.cache.AddAll(hist, startDate, date)
		if err != nil {
			return nil, fmt.Errorf("failed to add price history to cache: %w", err)
		}
		// TODO: deal with non-trading days and add them to the cache.
		// If we don't cache those, we'll always have to fetch data.
		result = append(result, hist[len(hist)-1])
	}
	return result, nil
}

func (yf *YFinance) FetchPriceHistory(symbol string, start, end time.Time) ([]*DailyQuote, error) {
	url, err := url.Parse("https://query2.finance.yahoo.com/v8/finance/chart/" + symbol)
	if err != nil {
		log.Fatalf("Cannot parse URL: %v", err)
	}
	q := url.Query()
	q.Set("interval", "1d")
	q.Set("events", "div,splits,capitalGains")
	q.Set("includePrePost", "false")
	q.Set("period1", fmt.Sprintf("%d", start.Unix()))
	q.Set("period2", fmt.Sprintf("%d", end.Unix()))
	q.Set("crumb", yf.cookieJar.Crumb)
	q.Set("formatted", "false")
	q.Set("corsDomain", "finance.yahoo.com")
	url.RawQuery = q.Encode()
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("User-Agent", userAgent)
	yf.cookieJar.AddCookies(req)
	log.Printf("Fetching data for %s/%v/%v from %s", symbol, start.Format(time.RFC3339), end.Format(time.RFC3339), url)
	resp, err := yf.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to get historic data failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	var yresp YFChartResponse
	if err := json.Unmarshal(body, &yresp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YFChartResponse: %w", err)
	}
	if yresp.Chart == nil {
		log.Printf("Unexpected response structure: %s", string(body))
		return nil, fmt.Errorf("YFChartResponse does not include chart")
	} else if yresp.Chart.Error != nil {
		e := yresp.Chart.Error
		if e.Code == "Not Found" {
			return nil, ErrTickerNotFound
		}
		return nil, fmt.Errorf("YFChartResponse contains an error: (code=%q, description=%q)", e.Code, e.Description)
	} else if len(yresp.Chart.Result) != 1 ||
		yresp.Chart.Result[0].Indicators == nil ||
		len(yresp.Chart.Result[0].Indicators.Quote) != 1 ||
		len(yresp.Chart.Result[0].Timestamps) < len(yresp.Chart.Result[0].Indicators.Quote[0].Close) {
		log.Printf("Unexpected response structure: %s", string(body))
		return nil, fmt.Errorf("YFChartResponse is missing expected data")
	}
	var hist []*DailyQuote
	var currency Currency
	res := yresp.Chart.Result[0]
	timezone := time.UTC
	if res.Meta != nil {
		currency = Currency(res.Meta.Currency)
		if tz, err := time.LoadLocation(res.Meta.ExchangeTimezoneName); err == nil {
			timezone = tz
		}
		if res.Meta.Symbol != symbol {
			return nil, fmt.Errorf("response is for a different symbol: requested %q, received %q", symbol, res.Meta.Symbol)
		}
	}
	for i, c := range res.Indicators.Quote[0].Close {
		hist = append(hist, &DailyQuote{
			Symbol:       symbol,
			Currency:     currency,
			ClosingPrice: Micros(c * 1e6),
			Timestamp:    time.Unix(int64(res.Timestamps[i]), 0).In(timezone),
		})
	}
	return hist, nil
}

// This is the second API that might be useful. Currently not used.
func PrintQuote(client *http.Client, ticker string, jar *CookieJar) error {
	url, err := url.Parse("https://query2.finance.yahoo.com/v10/finance/quoteSummary/" + ticker)
	if err != nil {
		return err
	}
	q := url.Query()
	modules := []string{
		// "financialData",
		"summaryDetail",
		"price",
		"quoteType",
	}
	q.Set("modules", strings.Join(modules, ","))
	q.Set("crumb", jar.Crumb)
	q.Set("symbol", ticker)
	q.Set("formatted", "false")
	q.Set("corsDomain", "finance.yahoo.com")
	url.RawQuery = q.Encode()
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Add("User-Agent", userAgent)
	for _, c := range jar.Cookies {
		req.AddCookie(&http.Cookie{
			Name:  c.Name,
			Value: c.Value,
		})
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return err
	}
	j, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(j))
	return nil
}

type quoteCacheKey struct {
	date   string
	symbol string
}
type quoteCacheValue struct {
	added time.Time
	quote *DailyQuote
}
type PriceHistoryCache struct {
	entries map[quoteCacheKey]quoteCacheValue
}

func NewPriceHistoryCache() *PriceHistoryCache {
	return &PriceHistoryCache{
		entries: make(map[quoteCacheKey]quoteCacheValue),
	}
}

func utcDate(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

func (c *PriceHistoryCache) AddAll(quotes []*DailyQuote, start, end time.Time) error {
	if start.After(end) {
		return fmt.Errorf("start after end: %v > %v", start, end)
	}
	if len(quotes) == 0 {
		return nil
	}
	hist := make([]*DailyQuote, len(quotes))
	copy(hist, quotes)
	slices.SortFunc(hist, func(a, b *DailyQuote) int {
		return a.Timestamp.Compare(b.Timestamp)
	})
	i := 0
	d := utcDate(start)
	end = utcDate(end)
	// Skip dates for which no history exists (e.g. if start is a holiday).
	first := utcDate(hist[0].Timestamp)
	for !d.After(end) && first.After(d) {
		d = d.AddDate(0, 0, 1)
	}
	// Add entries to cache.
	for !d.After(end) {
		// Advance i to point to the relevant hist entry
		for i < len(hist)-1 {
			hn := utcDate(hist[i+1].Timestamp)
			if hn.After(d) {
				break
			}
			i++
		}
		c.entries[quoteCacheKey{
			date:   d.Format("2006-01-02"),
			symbol: hist[i].Symbol,
		}] = quoteCacheValue{
			quote: hist[i],
			added: time.Now(),
		}
		d = d.AddDate(0, 0, 1)
	}
	return nil
}

func (c *PriceHistoryCache) Get(symbol string, date time.Time) (*DailyQuote, error) {
	key := quoteCacheKey{
		date:   date.Format("2006-01-02"),
		symbol: symbol,
	}
	val, ok := c.entries[key]
	if !ok {
		return nil, ErrNotCached
	}
	// Evict if:
	// - the entry was cached <12h after the entry's timestamp
	//   (which means the price might not be final),
	// - AND the entry was not added in the last N minutes.
	recentlyAdded := val.added.After(time.Now().Add(-10 * time.Minute))
	priceAge := val.added.Sub(val.quote.Timestamp)
	if priceAge < 12*time.Hour && !recentlyAdded {
		delete(c.entries, key)
		return nil, ErrNotCached
	}
	return val.quote, nil
}

// CSV Export

type DepotExportItem struct {
	QuantityMicros Micros   `json:"quantity"`
	WKN            string   `json:"wkn"`
	Currency       Currency `json:"currency"`
	PriceMicros    Micros   `json:"price"`
	ValueMicros    Micros   `json:"value"`
	ValueDate      Date     `json:"valueDate"`
}

func parseCSVFloat(s string) (Micros, error) {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	t := strings.TrimSuffix(s, "%")
	var p float64 = 1
	if t != s {
		p = 100
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, err
	}
	return Micros(math.Round(f / p * 1e6)), nil
}

func ReadDepotExportCSVFile(path string) ([]*DepotExportItem, error) {
	in, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open CSV file: %v", err)
	}
	defer in.Close()
	encIn := charmap.ISO8859_15.NewDecoder().Reader(in)
	return ReadDepotExportCSV(encIn)
}

func ReadDepotExportCSV(reader io.Reader) ([]*DepotExportItem, error) {
	r := csv.NewReader(reader)
	r.Comma = ';'
	firstRow := true
	colIdx := make(map[string]int)
	knownHdr := map[string]string{
		"Stück/Nom.":  "Quantity",
		"WKN":         "WKN",
		"Währung":     "Currency",
		"Akt. Kurs":   "Price",
		"Wert in EUR": "Value",
		"Datum":       "ValueDate",
	}
	var result []*DepotExportItem
	for {
		row, err := r.Read()
		if err == io.EOF || errors.Is(err, csv.ErrFieldCount) {
			// ErrFieldCount occurs if trailing rows in the export have fewer columns.
			break
		} else if err != nil {
			return nil, fmt.Errorf("error reading CSV file: %w", err)
		}
		if firstRow {
			for i, h := range row {
				if s, ok := knownHdr[h]; ok {
					colIdx[s] = i
				}
			}
			if len(colIdx) != len(knownHdr) {
				return nil, fmt.Errorf("not all expected headers present: %v", colIdx)
			}
			firstRow = false
			continue
		}
		qty, err := parseCSVFloat(row[colIdx["Quantity"]])
		if err != nil {
			return nil, fmt.Errorf("invalid quantity: %w", err)
		}
		price, err := parseCSVFloat(row[colIdx["Price"]])
		if err != nil {
			return nil, fmt.Errorf("invalid price: %w", err)
		}
		value, err := parseCSVFloat(row[colIdx["Value"]])
		if err != nil {
			return nil, fmt.Errorf("invalid value: %w", err)
		}
		valueDate, err := time.Parse("02.01.2006", row[colIdx["ValueDate"]])
		if err != nil {
			return nil, fmt.Errorf("invalid date: %w", err)
		}
		currency := row[colIdx["Currency"]]
		if !currencyRegexp.MatchString(currency) {
			return nil, fmt.Errorf("invalid currency: %s", currency)
		}
		result = append(result, &DepotExportItem{
			QuantityMicros: qty,
			WKN:            row[colIdx["WKN"]],
			Currency:       Currency(currency),
			PriceMicros:    price,
			ValueMicros:    value,
			ValueDate:      Date{valueDate},
		})
	}
	return result, nil
}
