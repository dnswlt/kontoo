package kontoo

import (
	"fmt"
	"strings"
	"time"
)

func FormatEntry(e *Entry, indent string) string {
	var b strings.Builder
	write := func(s string) {
		b.WriteString(indent)
		b.WriteString(s)
		b.WriteString("\n")
	}
	write(fmt.Sprintf("# %d", e.SequenceNum))
	write(e.Created.Format(time.DateTime))
	write(e.Type.String())
	write(fmt.Sprintf("%s \"%s\"", e.Asset.Id, e.Asset.Name))
	return b.String()
}
