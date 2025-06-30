package resources

import (
	"embed"
)

// Embedded resources
// Important: build the dist/ files with npm before building the Go binary!

//go:embed dist css images templates
var Files embed.FS
