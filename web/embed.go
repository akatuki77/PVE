// Package web はビルドした画面（web/dist）を実行ファイルに埋め込む。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist は埋め込んだ画面のファイルを、dist の中身がルートになる形で返す。
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
