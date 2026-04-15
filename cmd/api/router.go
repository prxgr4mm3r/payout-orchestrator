package main

import (
	"net/http"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/handlers"
)

func NewRouter(
	clientsHandler *handlers.ClientsHandler,
	fundingSourcesHandler *handlers.FundingSourcesHandler,
	authMW func(http.Handler) http.Handler,
) http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", healthz)

	protected := func(pattern string, handler http.HandlerFunc) {
		root.Handle(pattern, authMW(http.HandlerFunc(handler)))
	}

	protected("GET /clients/me", clientsHandler.GetCurrentClient)
	protected("POST /funding-sources", fundingSourcesHandler.CreateFundingSource)

	return root
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
