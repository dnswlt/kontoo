package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dnswlt/kontoo"
)

func ProcessAdd(args []string) error {
	path := "./ledger.json"
	store, err := kontoo.LoadStore(path)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	e, err := kontoo.ParseLedgerEntry(args)
	if err != nil {
		return fmt.Errorf("could not parse ledger entry: %w", err)
	}
	err = store.Add(e)
	if err != nil {
		return fmt.Errorf("could not add entry: %w", err)
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	store.Save()

	return nil
}

func ProcessServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 8084, "The port on which to listen")
	ledgerPath := fs.String("ledger", "./ledger.json", "Path to the ledger.json file")
	baseDir := fs.String("base-dir", ".", "Base directory from which static resources are served")
	debugMode := fs.Bool("debug", false, "Enable debug mode")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse error for serve flags: %w", err)
	}
	s, err := kontoo.NewServer(fmt.Sprintf("localhost:%d", *port), *ledgerPath, *baseDir)
	if err != nil {
		return err
	}
	s.DebugMode(*debugMode)
	return s.Serve()
}

func main() {
	if len(os.Args) == 1 {
		fmt.Printf("Please specify a valid command: [%s]\n",
			strings.Join([]string{"add", "serve", "import"}, ", "))
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
	default:
		err = fmt.Errorf("invalid command: %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
