package handlers

import (
	"errors"
	"net/http"
	"strconv"
)

var errInvalidPagination = errors.New("invalid pagination")

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

func paginationParams(r *http.Request, defaultLimit, maxLimit int32) (int32, int32, error) {
	limit, err := int32QueryParam(r, "limit", defaultLimit)
	if err != nil {
		return 0, 0, err
	}
	if limit <= 0 || limit > maxLimit {
		return 0, 0, errInvalidPagination
	}

	offset, err := int32QueryParam(r, "offset", 0)
	if err != nil {
		return 0, 0, err
	}
	if offset < 0 {
		return 0, 0, errInvalidPagination
	}

	return limit, offset, nil
}
