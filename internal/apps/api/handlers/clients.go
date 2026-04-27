package handlers

import (
	"encoding/json"
	"net/http"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/auth"
)

type ClientsHandler struct{}

func (h ClientsHandler) GetCurrentClient(w http.ResponseWriter, r *http.Request) {
	client, ok := apiauth.ClientFromContext(r.Context())
	if !ok {
		http.Error(w, "client not found in context", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(client); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
