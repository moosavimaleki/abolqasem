package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
)

func Start(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return Serve(listener)
}

func Serve(listener net.Listener) error {
	mux := http.NewServeMux()

	setupRoutes(mux)

	srv := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: mux,
	}

	log.Printf("Server listening on http://%s\n", listener.Addr().String())
	return srv.Serve(listener)
}
