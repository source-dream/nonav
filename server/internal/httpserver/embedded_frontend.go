package httpserver

import (
	"embed"
	"io/fs"
)

//go:embed web-dist web-dist/* web-dist/assets/*
var embeddedFrontendFS embed.FS

func getEmbeddedFrontendFS() fs.FS {
	sub, err := fs.Sub(embeddedFrontendFS, "web-dist")
	if err != nil {
		return nil
	}
	return sub
}
