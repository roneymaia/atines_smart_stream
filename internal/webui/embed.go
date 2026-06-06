package webui

import (
	"embed"
	"io/fs"
)

//go:embed assets/*
var files embed.FS

// FS returns the embedded UI asset filesystem rooted at "assets".
func FS() fs.FS {
	sub, err := fs.Sub(files, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
