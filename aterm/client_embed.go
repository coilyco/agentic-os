package main

import (
	"embed"
	"io/fs"
)

//go:embed all:clientdist
var embeddedClientFiles embed.FS

// embeddedClient is the client staged by `just aterm-client-embed`, nil when
// the build embedded none (only .gitkeep is tracked).
func embeddedClient() fs.FS {
	sub, err := fs.Sub(embeddedClientFiles, "clientdist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
