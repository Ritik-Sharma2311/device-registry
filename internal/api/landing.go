package api

import (
	_ "embed"
	"net/http"
)

//go:embed landing.html
var landingPage []byte

func landing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(landingPage)
}
