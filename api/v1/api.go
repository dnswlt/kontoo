// This file contains types for version v1 of the resource-oriented API.
package v1

import (
	"time"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/units"
)

//go:generate go-enum -type=AssetType -string -json -all=false
type AssetType int32

// Values for AssetType constants may change over time. Only the name
// should be assumed constant (and persisted in the JSON ledger).
const (
	UnspecifiedAssetType AssetType = iota

	Stock                   // Aktie
	StockExchangeTradedFund // Aktienfonds (ETF)
	StockMutualFund         // Aktienfonds (Investment)
	BondExchangeTradedFund  // Rentenfonds (ETF)
	BondMutualFund          // Rentenfonds (Investment)
	CorporateBond           // Unternehmensanleihe
	GovernmentBond          // Staatsanleihe
	FixedDepositAccount     // Festgeldkonto
	MoneyMarketAccount      // Tagesgeldkonto
	SavingsAccount          // Sparkonto
	CheckingAccount         // Girokonto
	BrokerageAccount        // Verrechnungskonto
	PensionAccount          // Altersvorsorgekonten (z.B. Säule 3a)
	Commodity               // Edelmetalle, Rohstoffe
	Cash                    // Bargeld
	TaxLiability            // Steuerschuld
	TaxPayment              // Steuer(voraus)zahlung
	CreditCardDebt          // Schulden auf Kreditkarte
	OtherDebt               // allg. Schulden
)

//go:generate go-enum -type=InterestPaymentSchedule -string -json -all=false
type InterestPaymentSchedule int32

const (
	UnspecifiedPayment InterestPaymentSchedule = iota
	AccruedPayment                             // Interest paid at maturity
	AnnualPayment                              // Interest paid yearly
)

type Asset struct {
	Created         time.Time
	Modified        time.Time
	Type            AssetType
	Name            string
	ShortName       string                  `json:",omitempty"`
	IssueDate       *civil.Date             `json:",omitempty"`
	MaturityDate    *civil.Date             `json:",omitempty"`
	InterestMicros  units.Micros            `json:"Interest,omitempty"`
	InterestPayment InterestPaymentSchedule `json:",omitempty"`
	IBAN            string                  `json:",omitempty"`
	AccountNumber   string                  `json:",omitempty"`
	ISIN            string                  `json:",omitempty"`
	WKN             string                  `json:",omitempty"`
	TickerSymbol    string                  `json:",omitempty"`
	// More ticker symbols, to get stock quotes online.
	// Keyed by quote service. Not used as ID.
	QuoteServiceSymbols map[string]string `json:",omitempty"`
	// (Optional) time zone in which the main exchange trading the equity is located.
	ExchangeTimezone string `json:",omitempty"`
	CustomID         string `json:",omitempty"`
	Currency         units.Currency
	Comment          string `json:",omitempty"`
}

//go:generate go-enum -type=EntryType -string -json -all=false
type EntryType int32

const (
	UnspecifiedEntryType EntryType = iota

	AssetPurchase
	AssetSale
	AssetPrice
	AssetHolding
	AccountCredit
	AccountDebit
	AccountBalance
	AssetMaturity
	DividendPayment
	InterestPayment
	ExchangeRate
)

type LedgerEntry struct {
	Created     time.Time
	SequenceNum int64
	ValueDate   civil.Date `json:",omitempty"`
	Type        EntryType  `json:",omitempty"`
	AssetRef    string     `json:",omitempty"`
	AssetID     string     `json:",omitempty"`

	Currency units.Currency `json:",omitempty"`

	// Only set for ExchangeRate type entries. Currency represents the base currency in that case.
	QuoteCurrency units.Currency `json:",omitempty"`

	// All *Micros fields are given in either micros of the currency or micros of a fraction.
	// 1'000'000 in ValueMicros equals 1.00 CHF (or whatever the Currency),
	// 500'000 PriceMicros of a bond equal a price of 50% of the nominal value.

	// Value in micros of the currency. For currency CHF, 1'000'000 ValueMicros equals 1 CHF.
	// Except for accounts, ValueMicros is only informational. The current value of other asset positions
	// is calculated from its QuantityMicros and its PriceMicros.
	ValueMicros    units.Micros `json:"Value,omitempty"`    // Account balance or asset value as calculated from quantity and price.
	QuantityMicros units.Micros `json:"Quantity,omitempty"` // Number of stocks, oz of gold, nominal value of a bond
	PriceMicros    units.Micros `json:"Price,omitempty"`    // Price of a single quantity of the asset. (1 * UnitValue) means 100% for prices specified in percent.
	CostMicros     units.Micros `json:"Cost,omitempty"`     // Cost incurred by the transaction.

	Comment string `json:",omitempty"`
}

func (a *Asset) Internal() *ledger.Asset {
	return &ledger.Asset{
		Created:             a.Created,
		Modified:            a.Modified,
		Type:                ledger.AssetType(a.Type),
		Name:                a.Name,
		ShortName:           a.ShortName,
		IssueDate:           a.IssueDate,
		MaturityDate:        a.MaturityDate,
		InterestMicros:      a.InterestMicros,
		InterestPayment:     ledger.InterestPaymentSchedule(a.InterestPayment),
		IBAN:                a.IBAN,
		AccountNumber:       a.AccountNumber,
		ISIN:                a.ISIN,
		WKN:                 a.WKN,
		TickerSymbol:        a.TickerSymbol,
		QuoteServiceSymbols: a.QuoteServiceSymbols,
		ExchangeTimezone:    a.ExchangeTimezone,
		CustomID:            a.CustomID,
		Currency:            a.Currency,
		Comment:             a.Comment,
	}
}

func FromInternalAsset(a *ledger.Asset) *Asset {
	return &Asset{
		Created:             a.Created,
		Modified:            a.Modified,
		Type:                AssetType(a.Type),
		Name:                a.Name,
		ShortName:           a.ShortName,
		IssueDate:           a.IssueDate,
		MaturityDate:        a.MaturityDate,
		InterestMicros:      a.InterestMicros,
		InterestPayment:     InterestPaymentSchedule(a.InterestPayment),
		IBAN:                a.IBAN,
		AccountNumber:       a.AccountNumber,
		ISIN:                a.ISIN,
		WKN:                 a.WKN,
		TickerSymbol:        a.TickerSymbol,
		QuoteServiceSymbols: a.QuoteServiceSymbols,
		ExchangeTimezone:    a.ExchangeTimezone,
		CustomID:            a.CustomID,
		Currency:            a.Currency,
		Comment:             a.Comment,
	}
}

func (e *LedgerEntry) Internal() *ledger.LedgerEntry {
	return &ledger.LedgerEntry{
		Created:        e.Created,
		SequenceNum:    e.SequenceNum,
		ValueDate:      e.ValueDate,
		Type:           ledger.EntryType(e.Type),
		AssetRef:       e.AssetRef,
		AssetID:        e.AssetID,
		Currency:       e.Currency,
		QuoteCurrency:  e.QuoteCurrency,
		ValueMicros:    e.ValueMicros,
		QuantityMicros: e.QuantityMicros,
		PriceMicros:    e.PriceMicros,
		CostMicros:     e.CostMicros,
		Comment:        e.Comment,
	}
}

func FromInternalEntry(e *ledger.LedgerEntry) *LedgerEntry {
	return &LedgerEntry{
		Created:        e.Created,
		SequenceNum:    e.SequenceNum,
		ValueDate:      e.ValueDate,
		Type:           EntryType(e.Type), // cast from internal to external
		AssetRef:       e.AssetRef,
		AssetID:        e.AssetID,
		Currency:       e.Currency,
		QuoteCurrency:  e.QuoteCurrency,
		ValueMicros:    e.ValueMicros,
		QuantityMicros: e.QuantityMicros,
		PriceMicros:    e.PriceMicros,
		CostMicros:     e.CostMicros,
		Comment:        e.Comment,
	}
}
