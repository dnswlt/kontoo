# kontoo -- accounting helper

Log transactions of your financial assets and generate reports.

## Installation

Make sure you have NodeJS and `npm` installed. Then run

```bash
npm run build
go run cmd/kontoo/kontoo.go serve -debug -ledger ./ledger.json
```

## Development

Run

```bash
npx webpack --watch
```

to automatically update `.js` and `.css` resources (still requires
a browser refresh).
