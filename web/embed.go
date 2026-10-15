// Package web embeds the dashboard so the server binary is self-contained.
package web

import "embed"

//go:embed index.html static
var FS embed.FS
