package server

import (
	"io/fs"
	"net/http"
)

var webFS fs.FS

func SetWebFS(f fs.FS) {
	webFS = f
}

func setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/state", handleAPIState)
	mux.HandleFunc("/api/sessions", handleAPISessions)
	mux.HandleFunc("/api/hook", handleAPIHook)
	mux.HandleFunc("/api/session/", handleAPISessionMessages)
	mux.HandleFunc("/api/events", handleAPIEvents)

	if webFS != nil {
		subFS, _ := fs.Sub(webFS, "web")
		mux.Handle("/", http.FileServer(http.FS(subFS)))
	} else {
		mux.Handle("/", http.FileServer(http.Dir("web")))
	}
}
