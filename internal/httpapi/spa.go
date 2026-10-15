package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the built client from fsys. Paths that do not name a
// file fall back to index.html, so the client can do its own routing.
func spaHandler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" {
			for _, seg := range strings.Split(name, "/") {
				if strings.HasPrefix(seg, ".") {
					http.NotFound(w, r)
					return
				}
			}
			if isFile(fsys, name) {
				if strings.HasPrefix(name, "static/") {
					// Vite content-hashes everything under static/.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				// fs.FS rejects ".." and absolute paths, and fsys is the embedded client.
				http.ServeFileFS(w, r, fsys, name) //nolint:gosec // G703: not a path on disk
				return
			}
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}

		if !isFile(fsys, "index.html") {
			http.Error(w, "web client is not built: run pnpm -C web build", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}

func isFile(fsys fs.FS, name string) bool {
	st, err := fs.Stat(fsys, name)
	return err == nil && !st.IsDir()
}
