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
	Id   string
	Type AssetType
	Name string
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
	EntryDeletion        EntryType = 7
)

type Entry struct {
	Created     time.Time
	SequenceNum int64
	ValueDate   time.Time
	Type        EntryType
	Asset       Asset

	Currency Currency

	// All *Micros fields are given in either micros of the currency or micros of a fraction.
	// 1'000'000 CHF in ValueMicros equal 1 CHF, 500'000 PriceMicros of a bond equal a price of 50% of the
	// nominal value.

	ValueMicros        int64 // Value in micros of the currency. 1'000'000 CHF in ValueMicros equal 1 CHF.
	NominalValueMicros int64 // Nominal value of a bond.
	QuantityMicros     int64 // Number of stocks, oz of gold.
	PriceMicros        int64 // Price of a single quantity of the asset.

	CostMicros int64 // Cost incurred by the transaction.

	RefSequenceNum int64 // SequenceNum of a previous entry that this one refers to (e.g. for deletion).
}

type Ledger struct {
	entries []*Entry
}

type AssetValue struct {
	Asset              Asset
	ValueDate          time.Time
	PriceDate          time.Time
	ValueMicros        int64
	NominalValueMicros int64 // Nominal value of a bond.
	QuantityMicros     int64 // Number of stocks.
	PriceMicros        int64 // Price of a single quantity of the asset.
}

const (
	Micros    = 1
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
