package server

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
)

type ledgerFilterOption struct {
	Value, Label string
	Selected     bool
}
type ledgerFilterChip struct{ Label, URL string }
type ledgerSortHeader struct{ URL, Indicator, AriaSort string }
type ledgerView struct {
	IncludeMarket                                           bool
	Search, From, Until, Sort, Date, Error                  string
	Assets                                                  []ledgerFilterOption
	Chips                                                   []ledgerFilterChip
	SortHeaders                                             map[string]ledgerSortHeader
	ClearURL                                                string
	EntryCount                                              int
	Page, PageCount, FirstEntry, LastEntry                  int
	FirstPageURL, PreviousPageURL, NextPageURL, LastPageURL string
}

const ledgerPageSize = 100

func ledgerURL(params url.Values) string {
	return newURL("/kontoo/ledger", params).String()
}
func copyParams(params url.Values) url.Values {
	copy := make(url.Values, len(params))
	for key, values := range params {
		copy[key] = slices.Clone(values)
	}
	return copy
}
func removeFilter(params url.Values, key, value string) string {
	copy := copyParams(params)
	copy.Del("page")
	copy[key] = slices.DeleteFunc(copy[key], func(v string) bool { return v == value })
	return ledgerURL(copy)
}

func (s *Server) ledgerViewContext(r *http.Request) (map[string]any, error) {
	params := copyParams(r.URL.Query())
	params.Del("snippet")
	params.Del("q")
	params.Del("mode")
	v := &ledgerView{
		Search: params.Get("search"), From: params.Get("from"), Until: params.Get("until"),
		Date: params.Get("date"), IncludeMarket: len(params["include_market"]) == 0 || slices.Contains(params["include_market"], "1"),
		SortHeaders: make(map[string]ledgerSortHeader),
		Page:        1, PageCount: 1,
	}
	if len(params["include_market"]) > 0 {
		if v.IncludeMarket {
			params.Set("include_market", "1")
		} else {
			params.Set("include_market", "0")
		}
	}
	for _, key := range []string{"type", "class", "currency"} {
		params.Del(key)
	}
	ctx := s.addCommonCtx(r, map[string]any{"Filters": v})
	var validationErr error
	fail := func(err error) {
		if validationErr == nil {
			validationErr = err
		}
	}
	if raw := params.Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			fail(fmt.Errorf("page must be a positive whole number"))
		} else {
			v.Page = page
		}
	}
	filters := ledger.EntryFilters{Search: v.Search, AssetIDs: slices.Clone(params["asset"]), HideMarketData: !v.IncludeMarket}
	assetNames := map[string]string{}
	assets := slices.Clone(s.store.AllAssets())
	slices.SortFunc(assets, func(a, b *ledger.Asset) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	for _, asset := range assets {
		assetNames[asset.ID()] = asset.Name
		v.Assets = append(v.Assets, ledgerFilterOption{asset.ID(), asset.Name + " · " + asset.ID(), slices.Contains(params["asset"], asset.ID())})
	}
	for _, id := range filters.AssetIDs {
		if _, ok := assetNames[id]; !ok {
			fail(fmt.Errorf("unknown asset: %s", id))
		}
	}
	for _, date := range []struct {
		raw, label string
		target     *civil.Date
	}{{v.From, "start", &filters.From}, {v.Until, "end", &filters.Until}} {
		if date.raw == "" {
			continue
		}
		parsed, err := civil.ParseDate(date.raw)
		if err != nil {
			fail(fmt.Errorf("invalid %s date: use YYYY-MM-DD", date.label))
			continue
		}
		*date.target = parsed
	}
	query, err := ledger.NewFilterQuery(filters)
	if err != nil {
		fail(err)
	}
	if query != nil {
		if sort := params.Get("sort"); sort != "" {
			if err := query.SetOrdering(sort); err != nil {
				fail(err)
			}
		}
		v.Sort = query.Ordering()
	}
	if validationErr == nil {
		rows := s.store.LedgerEntryRows(query)
		v.EntryCount = len(rows)
		v.PageCount = max(1, (v.EntryCount+ledgerPageSize-1)/ledgerPageSize)
		// Clamp stale page links after filtering or deleting the last entry on a page.
		v.Page = min(v.Page, v.PageCount)
		start := (v.Page - 1) * ledgerPageSize
		end := min(start+ledgerPageSize, v.EntryCount)
		ctx["TableRows"] = rows[start:end]
		if v.EntryCount > 0 {
			v.FirstEntry, v.LastEntry = start+1, end
		}
		pageURL := func(page int) string {
			pageParams := copyParams(params)
			if page == 1 {
				pageParams.Del("page")
			} else {
				pageParams.Set("page", strconv.Itoa(page))
			}
			return ledgerURL(pageParams)
		}
		if v.Page > 1 {
			v.FirstPageURL, v.PreviousPageURL = pageURL(1), pageURL(v.Page-1)
		}
		if v.Page < v.PageCount {
			v.NextPageURL, v.LastPageURL = pageURL(v.Page+1), pageURL(v.PageCount)
		}
	} else {
		v.Error = validationErr.Error()
	}

	base := make(url.Values)
	if v.Date != "" {
		base.Set("date", v.Date)
	}
	v.ClearURL = ledgerURL(base)
	if v.Search != "" {
		v.Chips = append(v.Chips, ledgerFilterChip{"Search: " + v.Search, removeFilter(params, "search", v.Search)})
	}
	for _, option := range v.Assets {
		if option.Selected {
			v.Chips = append(v.Chips, ledgerFilterChip{"Asset: " + assetNames[option.Value], removeFilter(params, "asset", option.Value)})
		}
	}
	for _, date := range []struct{ key, label, value string }{{"from", "From", v.From}, {"until", "Until", v.Until}} {
		if date.value != "" {
			v.Chips = append(v.Chips, ledgerFilterChip{date.label + ": " + date.value, removeFilter(params, date.key, date.value)})
		}
	}

	firstSort := strings.Split(v.Sort, ",")[0]
	for _, field := range []string{"num", "valuedate", "type", "assetid", "assetname", "class", "currency", "value", "cost", "quantity", "price", "totalvalue", "totalquantity", "totalcost", "comment"} {
		header := ledgerSortHeader{AriaSort: "none"}
		next := field
		if firstSort == field {
			header.Indicator, header.AriaSort, next = "↑", "ascending", "-"+field
		}
		if firstSort == "-"+field {
			header.Indicator, header.AriaSort = "↓", "descending"
		}
		if firstSort == "newest" && field == "valuedate" {
			header.Indicator, header.AriaSort = "↓", "descending"
		}
		if firstSort == "oldest" && field == "valuedate" {
			header.Indicator, header.AriaSort, next = "↑", "ascending", "-valuedate"
		}
		if field == "valuedate" {
			if strings.HasPrefix(next, "-") {
				next += ",-num"
			} else {
				next += ",num"
			}
		}
		sortParams := copyParams(params)
		sortParams.Del("page")
		sortParams.Set("sort", next)
		header.URL = ledgerURL(sortParams)
		v.SortHeaders[field] = header
	}
	return ctx, validationErr
}
