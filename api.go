package kontoo

import (
	"time"
)

type LogType int32

const (
	UnspecifiedLogType LogType = 0
	BuyTransaction     LogType = 1
	SellTransaction    LogType = 2
	AssetMaturity      LogType = 3
	DividendPayment    LogType = 4
	InterestPayment    LogType = 5
	AssetValue         LogType = 6
)

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
	PensionAccount          AssetType = 12 // Jegliche Altersvorsorgekonten (z.B. Säule 3a)
	OtherAssetType          AssetType = 999
)

type Asset struct {
	Id   string
	Type AssetType
	Name string
}

type Log struct {
	Created   time.Time
	ValueDate time.Time
	Type      LogType
	Asset     Asset
	Currency  string // Three-letter code, e.g. CHF, EUR, USD.

	// All *Micros fields are given in either micros of the currency or micros of a fraction.
	// 1'000'000 CHF in ValueMicros equal 1 CHF, 500'000 PriceMicros of a bond equal a price of 50% of the
	// nominal value.

	ValueMicros        int64 // Value in micros of the currency. 1'000'000 CHF in ValueMicros equal 1 CHF.
	NominalValueMicros int64 // Nominal value of a bond.
	QuantityMicros     int64 // Number of stocks.
	CostMicros         int64 // Cost incurred by the transaction.
	PriceMicros        int64 // Price of a single quantity of the asset.
}

const (
	Micros    = 1
	Millis    = 1_000
	UnitValue = 1_000_000
)
