package web

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler は画面のファイルを返す。存在しないパスには index.html を返す（/api/ は除く）。
func spaHandler(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(files, name); err != nil {
			index, err := fs.ReadFile(files, "index.html")
			if err != nil {
				http.Error(w, "画面がビルドされていません", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(index)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
