package kontoo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestSaveLoad(t *testing.T) {
	ref := NewLedger()
	ref.entries = []*Entry{
		{
			Created: time.Date(2023, 1, 1, 17, 0, 0, 0, time.UTC),
		},
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := ref.Save(path); err != nil {
		t.Fatalf("could not save ledger: %s", err)
	}
	l := NewLedger()
	l.Load(path)
	if diff := cmp.Diff(ref, l, cmp.AllowUnexported(Ledger{})); diff != "" {
		t.Errorf("Loaded ledger differs (-want +got):\n%s", diff)
	}
}

func TestSaveLoadEmpty(t *testing.T) {
	ref := NewLedger()
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := ref.Save(path); err != nil {
		t.Fatalf("could not save ledger: %s", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not load ledger file: %s", err)
	}
	if string(data) != "[]" {
		t.Errorf("expected empty list, got %s", data)
	}
}
