package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// contentTypes pins the MIME types of the built client. On Windows,
// mime.TypeByExtension reads the registry, where .js can be text/plain
// depending on installed software, and browsers refuse to run module scripts
// served with a non-JavaScript type.
var contentTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".html":  "text/html; charset=utf-8",
	".ico":   "image/x-icon",
	".jpeg":  "image/jpeg",
	".jpg":   "image/jpeg",
	".js":    "text/javascript; charset=utf-8",
	".json":  "application/json",
	".map":   "application/json",
	".mjs":   "text/javascript; charset=utf-8",
	".png":   "image/png",
	".svg":   "image/svg+xml",
	".txt":   "text/plain; charset=utf-8",
	".webp":  "image/webp",
	".woff":  "font/woff",
	".woff2": "font/woff2",
}

// setContentType sets Content-Type for name from contentTypes, so that
// http.ServeFileFS does not fall back to mime.TypeByExtension.
func setContentType(w http.ResponseWriter, name string) {
	if ct, ok := contentTypes[strings.ToLower(path.Ext(name))]; ok {
		w.Header().Set("Content-Type", ct)
	}
}

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
				setContentType(w, name)
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
		setContentType(w, "index.html")
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}

func isFile(fsys fs.FS, name string) bool {
	st, err := fs.Stat(fsys, name)
	return err == nil && !st.IsDir()
}
