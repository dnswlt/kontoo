package kontoo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type PriceHistoryItem struct {
	Date         time.Time
	ClosingPrice Micros
}

type PriceHistory struct {
	Ticker   string
	Currency string
	History  []PriceHistoryItem
}

type YFinance struct {
	client    *http.Client
	cookieJar CookieJar
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
}
type YFIndicators struct {
	Quote []*YFQuote `json:"quote"`
}
type YFQuote struct {
	Close []float64 `json:"close"`
}

// jq '{close: .chart.result[0].indicators.quote[0].close, timestamps: .chart.result[0].timestamp}' testdata/chart.json

const (
	userAgent       = `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15`
	cookieJarEnvVar = "YFCOOKIEJAR"
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
	yf := &YFinance{client: &http.Client{}}
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

func (yf *YFinance) GetPriceHistory(ticker string, start, end time.Time) (*PriceHistory, error) {
	url, err := url.Parse("https://query2.finance.yahoo.com/v8/finance/chart/" + ticker)
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
	q.Set("symbol", ticker)
	q.Set("formatted", "false")
	q.Set("corsDomain", "finance.yahoo.com")
	url.RawQuery = q.Encode()
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("User-Agent", userAgent)
	yf.cookieJar.AddCookies(req)
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
		len(yresp.Chart.Result[0].Timestamps) != len(yresp.Chart.Result[0].Indicators.Quote[0].Close) {
		log.Printf("Unexpected response structure: %s", string(body))
		return nil, fmt.Errorf("YFChartResponse is missing expected data")
	}
	hist := &PriceHistory{
		Ticker: ticker,
	}
	res := yresp.Chart.Result[0]
	timezone := time.UTC
	if res.Meta != nil {
		hist.Currency = res.Meta.Currency
		if tz, err := time.LoadLocation(res.Meta.ExchangeTimezoneName); err == nil {
			timezone = tz
		}
	}
	for i, c := range res.Indicators.Quote[0].Close {
		hist.History = append(hist.History, PriceHistoryItem{
			ClosingPrice: Micros(c * 1e6),
			Date:         time.Unix(int64(res.Timestamps[i]), 0).In(timezone),
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
