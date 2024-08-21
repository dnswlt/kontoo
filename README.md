# kontoo -- accounting helper

Log transactions of your financial assets and generate reports.

## Brainstorming

OK, so I could do all of this in Excel. Pros include:

* Visualisations are easy
* Pivot table functionality and other slicing and dicing
* User interface to enter data

So why would I want to do this as a command-line application?

* I can generate and test(!) my own reports, e.g. as HTML.
* I can base all reports and statistics on a single data structure: the ledger
  I don't need to manually curate my quarterly reports, asset allocations, etc.
  I maintain a single ledger from which all of this is derived automatically.
* I can generate reports any time, not only quarterly.

Success criteria:

* MUST HAVE formatted, storable and printable tables. A console output is not sufficient.
  * The natural solution to this is to generate HTML reports.
* SHOULD HAVE visualisation (bar / donut charts, plots).
* MUST be easy to edit: both for corrections of previous data and to add new data.
  * Either a fancy UI should support me in entering the data with combo boxes etc.
  * Or the text-based interface should support human-friendly names for accounts and assets.
    I don't want to have to type each ISIN in full when entering the current value of an asset,
    for example.
*

### Ledger playground

Data is stored in two data structures: the asset metadata table and the ledger.

Asset metadata example:

```json
{
    "ISIN": "DE0001141828",
    "Name": "Bund.Deutschland Bundesobl.Ser.182 v.2020(25)",
    "ShortName": "Bund.182",
    "AssetType": "GovernmentBond",
    "IssueDate": "2020-07-10",
    "MaturityDate": "2025-10-10",
    "Interest": "0.0%",
}
```

This can be stored as a JSON list in a single text file `assets.json`.

The ledger is a sequence of entries. Entries represent sales and purchases
of assets, current account balances, current values of assets, etc.

Ledger entry examples:

```json
{
    "Created": "2024-07-08T17:23:59+0200",
    "SequenceNum": 1001,
    "ValueDate": "2024-07-09",
    "Type": "AssetPurchase",
    "Currency": "EUR",
    "ValueMicros": "980.00",
    "NominalValueMicros": "1000.00",
    "AssetRef": "DE0001141828",
    "Comment": "Bond purchase example",
}
{
    "Created": "2024-07-08T17:23:59+0200",
    "SequenceNum": 1001,
    "ValueDate": "2024-07-09",
    "Type": "AccountBalance",
    "Currency": "EUR",
    "ValueMicros": "10000.00",
    "AssetRef": "DE12312312323",
    "Comment": "Account balance example",
}
```

An AssetRef is resolved in this order:

1. A dedicated "ID" attribute of an asset.
2. The "IBAN" attribute.
3. The "AccountNumber" attribute.
4. The "ISIN" attribute.
5. The "WKN" attribute.
6. The "TickerSymbol" attribute.

To enter a new entry into the ledger, you don't want to type all the above.
In particular:

* The "Created" date should be added automatically.
* The "AssetRef" must be checked against the metadata and has to exist.
  * Short names should be supported, no one wants to enter an ISIN.
* The "SequenceNum" should be added automatically.
* Inserting data in JSON is not very convenient.

An idea for a simpler workflow would be:

You can add entries on the command line:

```bash
kontoo add buy Bund.182 -v EUR 980 -n EUR 1000 -c 'Bond purchase example'
kontoo add balance MeinKonto -v EUR 10000 -c 'Account balance example'
```
