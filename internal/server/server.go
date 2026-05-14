package server

import (
	"fmt"
	"log"
	"net/http"
)

func Start(port int) error {
	mux := http.NewServeMux()

	setupRoutes(mux)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("Server listening on http://%s\n", addr)
	return srv.ListenAndServe()
}
