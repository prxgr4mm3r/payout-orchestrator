package handlers

import (
	"net/http"
	"strconv"
)

func int32QueryParam(r *http.Request, name string, defaultValue int32) (int32, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, err
	}

	return int32(value), nil
}
