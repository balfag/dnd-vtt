// Package web embeds the built browser client into the server binary.
package web

import (
	"embed"
	"io/fs"
)

// dist holds the output of `pnpm -C web build`. The all: prefix also embeds
// dist/.gitkeep, so the package compiles before the client has been built.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built client with dist/ as its root.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// Unreachable: "dist" is a valid path that is always embedded.
		panic(err)
	}
	return sub
}
