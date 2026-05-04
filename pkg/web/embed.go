package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var embeddedFiles embed.FS

// staticFiles is an http.FileSystem rooted at the embedded static/ directory.
var staticFiles = func() http.FileSystem {
	sub, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}()
