# kontoo -- accounting helper

Log transactions of your financial assets and generate reports.

## Installation

Make sure you have NodeJS and `npm` installed. Then run

```bash
npm run build
go run ./cmd/kontoo serve -debug -ledger ./ledger.json
```

## Development

Run

```bash
npx webpack --watch
```

to automatically update `.js` and `.css` resources (still requires
a browser refresh).

## Notes

For equities and bonds, ledger entries are simple:

* AssetPurchase with Price, Quantity, and Cost.
* AssetSale with the same.
* AssetHolding to assert a specific position holding, with Price, Quantity, and optional Cost.
* AssetPrice to assert a specific (market) price for a single unit of the asset.

Bonds have an additional entry type signifying their maturity:

* AssetMaturity to assert that the asset has matured. Its position will be zero afterwards.

For checking and savings accounts, things are different, but also simple:

* AccountBalance to assert the balance (value) of an account.
* AccountCredit to add to the balance.
* AccountDebit to subtract from the balance.

Only the Value of the asset matters. These account types have neither Cost, Price, nor Quantity.
AccountCredit and AccountDebit are rare and are mostly allowed here "for completeness"
(they are more common for other asset types like TaxPayment).
Most checking and savings accounts will only have AccountBalance entries.

Fixed deposit accounts are a bit weirder. They are maturing and yield interest just like
bonds. However, since they are not traded on an exchange, they have no Price or Cost.
Their value tends to increase over time (in the case of accumulating interest).

The "natural" entry types are:

* AccountCredit to add to the balance.
* AccountBalance to assert the balance (value) of the fixed deposit account.
* AssetMaturity when the fixed deposit account has matured.

To calculate the total earnings of the investment, we need to sum up the individual
account credits (typically a single one at the beginning, since each new investment
results in a new fixed deposit account) and subtract them from the final balance at
maturity.

We should therefore enforce the following protocol for fixed deposit accounts:

* AccountCredit to add to the balance. In particular, this entry type MUST be used
  as the initial entry type when the deposit account is opened.
* AccountBalance to assert the balance (value) of the fixed deposit account at any
  point in time after the initial AccountCredit.
* AssetMaturity at maturity. A Value MUST be specified, which specifies the final
  account balance.
