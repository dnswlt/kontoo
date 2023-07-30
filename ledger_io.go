package kontoo

import (
	"encoding/json"
	"os"
)

func (l *Ledger) Save(path string) error {
	data, err := json.MarshalIndent(l.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (l *Ledger) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &l.entries)
}
