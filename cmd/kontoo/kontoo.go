package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/dnswlt/kontoo/pkg/kontoo"
)

func ProcessAdd(args []string) error {
	if len(args) == 0 || strings.TrimLeft(args[0], "-") == "help" {
		lines := []string{
			"Usage of add:",
			"  add <ledger-path> (-<Field> <Value>)...",
		}
		fmt.Fprint(os.Stderr, strings.Join(lines, "\n"))
		return flag.ErrHelp
	}
	ledgerPath := args[0]
	store, err := kontoo.LoadStore(ledgerPath)
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}
	e, err := kontoo.ParseLedgerEntry(args[1:])
	if err != nil {
		return fmt.Errorf("could not parse ledger entry: %w", err)
	}
	err = store.Add(e)
	if err != nil {
		return fmt.Errorf("could not add entry: %w", err)
	}
	store.Save()

	return nil
}

func ProcessCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	baseCurrency := fs.String("base-currency", "EUR", "Base currency (3-letter ISO code)")
	path := fs.String("path", "", "Path to write the created ledger to. If empty, the ledger is written to stdout.")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("flag parse error: %w", err)
	}
	ccy := kontoo.Currency(*baseCurrency)
	if !kontoo.ValidCurrency(ccy) {
		return fmt.Errorf("invalid currency %q", *baseCurrency)
	}
	l := kontoo.NewLedger(ccy)
	var out io.Writer
	if *path == "" {
		out = os.Stdout
	} else {
		fOut, err := os.OpenFile(*path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("cannot create ledger file: %w", err)
		}
		defer fOut.Close()
		out = fOut
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	err := enc.Encode(l)
	if err != nil {
		log.Fatalf("Cannot marshal ledger JSON: %v", err)
	}
	return nil
}

func ProcessServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 8084, "The port on which to listen")
	ledgerPath := fs.String("ledger", "./ledger.json", "Path to the ledger.json file")
	baseDir := fs.String("base-dir", "", `Directory for static resources ("" to use embedded resources)`)
	debugMode := fs.Bool("debug", false, "Enable debug mode (e.g. dynamic resource reload)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("flag parse error: %w", err)
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("extraneous args: %v", strings.Join(fs.Args(), " "))
	}
	if *debugMode && *baseDir == "" {
		return fmt.Errorf("must specify -base-dir if -debug is true")
	}
	s, err := kontoo.NewServer(fmt.Sprintf("localhost:%d", *port), *ledgerPath, *baseDir)
	if err != nil {
		return err
	}
	s.DebugMode(*debugMode)
	return s.Serve()
}

func main() {
	commands := []string{"add", "serve", "import", "create"}
	if len(os.Args) == 1 {
		fmt.Printf("Please specify a valid command: [%s]\n",
			strings.Join(commands, ", "))
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "add":
		err = ProcessAdd(os.Args[2:])
	case "serve":
		err = ProcessServe(os.Args[2:])
	case "import":
		err = ProcessImport(os.Args[2:])
	case "create":
		err = ProcessCreate(os.Args[2:])
	default:
		err = fmt.Errorf("invalid command: %q (valid values are [%s])",
			os.Args[1], strings.Join(commands, ", "))
	}
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
