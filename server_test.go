package kontoo

import "testing"

func TestLoadTemplates(t *testing.T) {
	s := &Server{
		baseDir: ".",
	}
	if err := s.reloadTemplates(); err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}
}
