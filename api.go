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
	Cash                    AssetType = 14 // Bargeld
	TaxLiability            AssetType = 15 // Steuerschuld
	GeneralDebt             AssetType = 16 // Allg. Schulden
	OtherAssetType          AssetType = 999
)

type Asset struct {
	Type           AssetType
	Name           string
	ShortName      string `json:",omitempty"`
	IssueDate      *Date  `json:",omitempty"`
	MaturityDate   *Date  `json:",omitempty"`
	InterestMicros Micros `json:",omitempty"`
	IBAN           string `json:",omitempty"`
	AccountNumber  string `json:",omitempty"`
	ISIN           string `json:",omitempty"`
	WKN            string `json:",omitempty"`
	TickerSymbol   string `json:",omitempty"`
	CustomID       string `json:",omitempty"`
	Currency       Currency
	AssetGroup     string `json:",omitempty"`
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
	AssetPrice           EntryType = 6
	AccountBalance       EntryType = 7
	ExchangeRate         EntryType = 8
)

// Dates without a time component.
type Date struct {
	time.Time
}

type LedgerEntry struct {
	Created     time.Time
	SequenceNum int64
	ValueDate   Date      `json:",omitempty"`
	Type        EntryType `json:",omitempty"`
	AssetRef    string    `json:",omitempty"`
	AssetID     string    `json:",omitempty"`

	Currency Currency `json:",omitempty"`

	// Only set for ExchangeRate type entries. Currency represents the base currency in that case.
	QuoteCurrency Currency `json:",omitempty"`

	// All *Micros fields are given in either micros of the currency or micros of a fraction.
	// 1'000'000 in ValueMicros equals 1.00 CHF (or whatever the Currency),
	// 500'000 PriceMicros of a bond equal a price of 50% of the nominal value.

	// Value in micros of the currency. 1'000'000 CHF in ValueMicros equal 1 CHF.
	// ValueMicros is only informational. The current value of any asset position
	// is calculated from its NominalValueMicros or QuantityMicros, and its price.
	// For account balances, the price is zero and only the nominal value is used.
	ValueMicros        Micros `json:"Value,omitempty"`
	NominalValueMicros Micros `json:"NominalValue,omitempty"` // Nominal value of a bond or balance of an account.
	QuantityMicros     Micros `json:"Quantity,omitempty"`     // Number of stocks, oz of gold.
	PriceMicros        Micros `json:"Price,omitempty"`        // Price of a single quantity of the asset.
	CostMicros         Micros `json:"Cost,omitempty"`         // Cost incurred by the transaction.

	Comment string `json:",omitempty"`
}

type LedgerHeader struct {
	BaseCurrency Currency `json:",omitempty"`
}

type AssetGroup struct {
	ID   string
	Name string
}

type Ledger struct {
	Header      *LedgerHeader  `json:",omitempty"`
	Assets      []*Asset       `json:",omitempty"`
	AssetGroups []*AssetGroup  `json:",omitempty"`
	Entries     []*LedgerEntry `json:",omitempty"`
}

type PVal struct {
	NominalValueMicros Micros
	QuantityMicros     Micros
	PriceMicros        Micros
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
