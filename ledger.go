package kontoo

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func DateVal(year int, month time.Month, day int) Date {
	return Date(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}

func (d *Date) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte("\"" + time.Time(d).Format("2006-01-02") + "\""), nil
}

func (d Date) Equal(e Date) bool {
	return time.Time(d).Equal(time.Time(e))
}

type Store struct {
	L        *Ledger
	assetMap map[string]*Asset // Maps the ledger's assets by ID.
	path     string            // Path to the ledger JSON.
}

func LoadStore(path string) (*Store, error) {
	l := &Ledger{}
	if err := l.Load(path); err != nil {
		return nil, fmt.Errorf("failed to load ledger: %w", err)
	}
	return NewStore(l, path)
}

func NewStore(ledger *Ledger, path string) (*Store, error) {
	m := make(map[string]*Asset)
	for _, asset := range ledger.Assets {
		id := asset.ID()
		if _, found := m[id]; found {
			return nil, fmt.Errorf("duplicate ID in ledger assets: %q", id)
		}
		m[asset.ID()] = asset
	}
	return &Store{
		L:        ledger,
		assetMap: m,
		path:     path,
	}, nil
}

func (s *Store) Save() error {
	return s.L.Save(s.path)
}

func (a *Asset) ID() string {
	if a.ISIN != "" {
		return a.ISIN
	}
	if a.IBAN != "" {
		return a.IBAN
	}
	if a.AccountNumber != "" {
		return a.AccountNumber
	}
	if a.WKN != "" {
		return a.WKN
	}
	if a.TickerSymbol != "" {
		return a.TickerSymbol
	}
	return ""
}

func (a *Asset) MatchRef(ref string) bool {
	return a.IBAN == ref || a.ISIN == ref || a.WKN == ref ||
		a.AccountNumber == ref || a.TickerSymbol == ref ||
		a.ShortName == ref || a.Name == ref
}

func (s *Store) NextSequenceNum() int64 {
	if len(s.L.Entries) == 0 {
		return 0
	}
	return s.L.Entries[len(s.L.Entries)-1].SequenceNum + 1
}

func (s *Store) FindAssetByRef(ref string) (*Asset, bool) {
	var res *Asset
	for _, asset := range s.L.Assets {
		if asset.MatchRef(ref) {
			if res != nil {
				// Non-unique reference
				return nil, false
			}
			res = asset
		}
	}
	if res == nil {
		return nil, false
	}
	return res, true
}

func (s *Store) Add(e *LedgerEntry) error {
	var a *Asset
	found := false
	if e.AssetID != "" {
		a, found = s.assetMap[e.AssetID]
	} else {
		a, found = s.FindAssetByRef(e.AssetRef)
	}
	if !found {
		return fmt.Errorf("no asset found for AssetRef %q", e.AssetRef)
	}
	if time.Time(e.ValueDate).IsZero() {
		return fmt.Errorf("ValueDate must be set")
	}
	e.Created = time.Now()
	e.SequenceNum = s.NextSequenceNum()
	// Change soft-link to ID ref:
	e.AssetRef = ""
	e.AssetID = a.ID()
	s.L.Entries = append(s.L.Entries, e)
	return nil
}

func (l *Ledger) Save(path string) error {
	data, err := l.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (l *Ledger) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, l)
}

func (l *Ledger) Marshal() ([]byte, error) {
	return json.MarshalIndent(l, "", "  ")
}
