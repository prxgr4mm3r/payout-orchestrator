package main

import (
	"log"
	"net/http"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/handlers"
)

func main() {

	fundingSourcesHandler := handlers.FundingSourcesHandler{}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /funding-sources", fundingSourcesHandler.RegisterFundingSource)
	mux.HandleFunc("POST /payouts", http.NotFoundHandler().ServeHTTP)
	mux.HandleFunc("GET /payouts/{id}", http.NotFoundHandler().ServeHTTP)
	mux.HandleFunc("GET /payouts", http.NotFoundHandler().ServeHTTP)

	srv := &http.Server{ 
		Addr: ":8080",
		Handler: mux,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}