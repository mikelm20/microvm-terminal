package api

import (
	"encoding/json"
	"net/http"

	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiError writes an apitypes.ApiError body. The request_id is pulled from
// the request context so clients can correlate with server logs.
func apiError(w http.ResponseWriter, r *http.Request, status int, code string, message string) {
	writeJSON(w, status, apitypes.ApiError{
		Code:      code,
		Message:   message,
		RequestID: requestIDFrom(r.Context()),
	})
}

func apiErrorRetry(w http.ResponseWriter, r *http.Request, status int, code string, message string, retryAfter int) {
	writeJSON(w, status, apitypes.ApiError{
		Code:              code,
		Message:           message,
		RetryAfterSeconds: &retryAfter,
		RequestID:         requestIDFrom(r.Context()),
	})
}

func decodeJSON(r *http.Request, out any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}
