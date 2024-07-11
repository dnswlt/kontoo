package kontoo

import (
	"time"
)

//go:generate go-enum -type=AssetType -string -json -all=false
type AssetType int32

const (
	UnspecifiedAssetType    AssetType = 0
	Stock                   AssetType = 1  // Aktie
	StockExchangeTradedFund AssetType = 2  // Aktienfonds (ETF)
	StockMutualFund         AssetType = 3  // Aktienfonds (Investment)
	BondExchangeTradedFund  AssetType = 4  // Rentenfonds (ETF)
	BondMutualFund          AssetType = 5  // Rentenfonds (Investment)
	CorporateBond           AssetType = 6  // Unternehmensanleihe
	GovernmentBond          AssetType = 7  // Staatsanleihe
	FixedDepositAccount     AssetType = 8  // Festgeldkonto
	MoneyMarketAccount      AssetType = 9  // Tagesgeldkonto
	SavingsAccount          AssetType = 10 // Sparkonto
	CheckingAccount         AssetType = 11 // Girokonto
	PensionAccount          AssetType = 12 // Altersvorsorgekonten (z.B. Säule 3a)
	Commodity               AssetType = 13 // Edelmetalle, Rohstoffe
	OtherAssetType          AssetType = 999
)

type Asset struct {
	Type           AssetType
	Name           string
	ShortName      string
	IssueDate      time.Time
	MaturityDate   time.Time
	InterestMicros Micros
	IBAN           string
	AccountNumber  string
	ISIN           string
	WKN            string
	TickerSymbol   string
}

//go:generate go-enum -type=EntryType -string -json -all=false
type EntryType int32

const (
	UnspecifiedEntryType EntryType = 0
	BuyTransaction       EntryType = 1
	SellTransaction      EntryType = 2
	AssetMaturity        EntryType = 3
	DividendPayment      EntryType = 4
	InterestPayment      EntryType = 5
	AssetValueStatement  EntryType = 6
	AccountBalance       EntryType = 8
)

// Dates without a time component.
type Date time.Time

type LedgerEntry struct {
	Created     time.Time
	SequenceNum int64     `json:",omitempty"`
	ValueDate   Date      `json:",omitempty"`
	Type        EntryType `json:",omitempty"`
	AssetRef    string    `json:",omitempty"`
	AssetID     string    `json:",omitempty"`

	Currency Currency `json:",omitempty"`

	// All *Micros fields are given in either micros of the currency or micros of a fraction.
	// 1'000'000 in ValueMicros equals 1.00 CHF (or whatever the Currency),
	// 500'000 PriceMicros of a bond equal a price of 50% of the nominal value.
	ValueMicros        Micros `json:"Value,omitempty"`        // Value in micros of the currency. 1'000'000 CHF in ValueMicros equal 1 CHF.
	NominalValueMicros Micros `json:"NominalValue,omitempty"` // Nominal value of a bond.
	QuantityMicros     Micros `json:"Quantity,omitempty"`     // Number of stocks, oz of gold.
	PriceMicros        Micros `json:"Price,omitempty"`        // Price of a single quantity of the asset.
	CostMicros         Micros `json:"Cost,omitempty"`         // Cost incurred by the transaction.

	Comment string `json:",omitempty"`
}

type Ledger struct {
	Assets  []*Asset       `json:",omitempty"`
	Entries []*LedgerEntry `json:",omitempty"`
}

type AssetValue struct {
	ValueMicros        Micros
	NominalValueMicros Micros // Nominal value of a bond.
	QuantityMicros     Micros // Number of stocks.
	PriceMicros        Micros // Price of a single quantity of the asset.
}

const (
	Millis    = 1_000
	UnitValue = 1_000_000
)

// Three-letter code, e.g. CHF, EUR, USD.
type Currency string

const (
	CHF Currency = "CHF"
	USD Currency = "USD"
	EUR Currency = "EUR"
)
