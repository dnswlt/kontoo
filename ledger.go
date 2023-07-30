package kontoo

import (
	"fmt"
	"math/big"
	"sort"
	"time"
)

func NewLedger() *Ledger {
	return &Ledger{
		entries: []*Entry{},
	}
}

type LedgerUpdate interface {
	EntryType() EntryType
}

type TransactionParams struct {
	txType             EntryType
	valueDate          time.Time
	asset              Asset
	currency           Currency
	priceMicros        int64
	quantityMicros     int64
	nominalValueMicros int64
	valueMicros        int64
	costMicros         int64
}

func (p *TransactionParams) EntryType() EntryType {
	return p.txType
}

func ValidateTransactionParams(p *TransactionParams) error {
	if p.asset.isNominalValueBased() {
		if p.nominalValueMicros <= 0 {
			return fmt.Errorf("nominal value must be greater than zero")
		}
	}
	if p.asset.isQuantityBased() {
		if p.quantityMicros <= 0 {
			return fmt.Errorf("quantity must be greater than zero")
		}
	}
	return nil
}

// Returns the result of multiplying two values expressed in micros.
// E.g., a == 2_000_000, b == 3_000_000 ==> MultMicros(a, b) == 6_000_000.
func Mmul(a int64, b int64) int64 {
	bigA := big.NewInt(a)
	bigB := big.NewInt(b)
	bigA.Mul(bigA, bigB)
	bigA.Div(bigA, big.NewInt(1_000_000))
	if !bigA.IsInt64() {
		panic(fmt.Sprintf("cannot represent %v as int64 micros", bigA))
	}
	return bigA.Int64()
}

func (a *Asset) isQuantityBased() bool {
	return a.Type == Stock || a.Type == StockExchangeTradedFund || a.Type == StockMutualFund
}

func (a *Asset) isNominalValueBased() bool {
	return a.Type == CorporateBond || a.Type == GovernmentBond
}

func (l *Ledger) AddTransaction(params *TransactionParams) (*Entry, error) {
	if params.txType != SellTransaction && params.txType != BuyTransaction {
		return nil, fmt.Errorf("not a valid transaction type: %v", params.txType)
	}
	seq := int64(len(l.entries))
	e := &Entry{
		Created:        time.Now(),
		SequenceNum:    seq,
		ValueDate:      params.valueDate,
		Type:           params.txType,
		Asset:          params.asset,
		Currency:       params.currency,
		ValueMicros:    params.valueMicros,
		QuantityMicros: params.quantityMicros,
		CostMicros:     params.costMicros,
		PriceMicros:    params.priceMicros,
	}
	l.entries = append(l.entries, e)
	return e, nil
}

func (l *Ledger) AssetIds() []string {
	m := make(map[string]struct{})
	var ids []string
	for _, e := range l.entries {
		id := e.Asset.Id
		if id == "" {
			continue
		}
		if _, found := m[id]; found {
			continue
		}
		m[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func (l *Ledger) Entries(assetId string, t time.Time) []*Entry {
	var entries []*Entry
	del := make(map[int64]bool)
	for i := len(l.entries) - 1; i >= 0; i-- {
		e := l.entries[i]
		if e.Asset.Id != assetId {
			continue
		}
		if e.Type == EntryDeletion {
			del[e.RefSequenceNum] = true
		}
		if e.ValueDate.After(t) || del[e.SequenceNum] {
			continue
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		c := entries[i].ValueDate.Compare(entries[j].ValueDate)
		if c == 0 {
			return entries[i].SequenceNum < entries[j].SequenceNum
		}
		return c < 0
	})
	return entries
}

func (v *AssetValue) Add(e *Entry) {
	v.QuantityMicros += e.QuantityMicros
	v.NominalValueMicros += e.NominalValueMicros
}

func (v *AssetValue) Subtract(e *Entry) {
	v.QuantityMicros -= e.QuantityMicros
	v.NominalValueMicros -= e.NominalValueMicros
}

func (v *AssetValue) Set(e *Entry) {
	v.QuantityMicros = e.QuantityMicros
	v.NominalValueMicros = e.NominalValueMicros
	v.ValueMicros = e.ValueMicros
	v.PriceMicros = e.PriceMicros
	v.PriceDate = e.ValueDate
}

func (v *AssetValue) Reset() {
	v.QuantityMicros = 0
	v.NominalValueMicros = 0
	v.ValueMicros = 0
	v.PriceMicros = 0
	v.PriceDate = time.Time{}
}

func (l *Ledger) AssetValue(assetId string, t time.Time) AssetValue {
	entries := l.Entries(assetId, t)
	if len(entries) == 0 {
		return AssetValue{
			Asset: Asset{
				Id: assetId,
			},
		}
	}
	var val AssetValue
	ref := entries[len(entries)-1]
	val.Asset = ref.Asset
	val.ValueDate = ref.ValueDate
	for _, e := range entries {
		if e.PriceMicros > 0 {
			val.PriceMicros = e.PriceMicros
			val.PriceDate = e.ValueDate
		}
		switch e.Type {
		case BuyTransaction:
			val.Add(e)
		case SellTransaction:
			val.Subtract(e)
		case AccountBalance, AssetValueStatement:
			val.Set(e)
		case AssetMaturity:
			val.Reset()
		}
	}
	return val
}

/*
const (
	UnspecifiedEntryType EntryType = 0
	BuyTransaction       EntryType = 1
	SellTransaction      EntryType = 2
	AssetMaturity        EntryType = 3
	DividendPayment      EntryType = 4
	InterestPayment      EntryType = 5
	AssetValueStatement  EntryType = 6
	AccountBalance       EntryType = 8
	EntryDeletion        EntryType = 7

)
*/
