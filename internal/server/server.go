package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
)

const localServerHost = "127.0.0.1"

func Start(port int) error {
	listener, err := net.Listen("tcp", localServerAddress(port))
	if err != nil {
		return err
	}
	return Serve(listener)
}

func localServerAddress(port int) string {
	return net.JoinHostPort(localServerHost, strconv.Itoa(port))
}

func Serve(listener net.Listener) error {
	mux := http.NewServeMux()

	setupRoutes(mux)
	workspaceTelegramBridge.Reload()
	stopCodexManagerMaintenance := startCodexManagerMaintenance(context.Background())
	defer stopCodexManagerMaintenance()
	stopCodexManagerSessionMonitor := startCodexManagerSessionMonitor(context.Background())
	defer stopCodexManagerSessionMonitor()
	stopCodexManagerChromeScan := startCodexManagerChromeScanWorker(context.Background())
	defer stopCodexManagerChromeScan()

	srv := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: mux,
	}

	log.Printf("Server listening on http://%s\n", listener.Addr().String())
	return srv.Serve(listener)
}
