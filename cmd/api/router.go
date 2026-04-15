package main

import (
	"net/http"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/handlers"
)

func NewRouter(
	clientsHandler *handlers.ClientsHandler,
	authMW func(http.Handler) http.Handler,
) http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", healthz)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /clients/me", clientsHandler.GetCurrentClient)

	root.Handle("GET /clients/", authMW(protected))

	return root
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
