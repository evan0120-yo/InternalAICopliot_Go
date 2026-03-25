package infra

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// APIError is the frontend-visible error body.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIResponse is the shared HTTP envelope.
type APIResponse struct {
	Success bool      `json:"success"`
	Data    any       `json:"data,omitempty"`
	Error   *APIError `json:"error,omitempty"`
}

// WriteJSON writes a success response with a stable JSON envelope.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	})
}

// WriteError writes an error response with a stable JSON envelope.
func WriteError(w http.ResponseWriter, err error) {
	businessErr := AsBusinessError(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(businessErr.HTTPStatus)
	_ = json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error: &APIError{
			Code:    businessErr.Code,
			Message: businessErr.Message,
		},
	})
}

const DefaultJSONBodyLimitBytes int64 = 1 << 20

// DecodeJSONStrict decodes JSON, rejects unknown fields, and enforces a body size limit.
func DecodeJSONStrict(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultJSONBodyLimitBytes
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return NewError("REQUEST_BODY_TOO_LARGE", "Request body exceeds the allowed size limit.", http.StatusRequestEntityTooLarge)
		}
		if errors.Is(err, io.EOF) {
			return NewError("INVALID_JSON", "Request body is required.", http.StatusBadRequest)
		}
		return NewError("INVALID_JSON", "Request body is not valid JSON.", http.StatusBadRequest)
	}
	if decoder.More() {
		return NewError("INVALID_JSON", "Request body must contain a single JSON object.", http.StatusBadRequest)
	}
	return nil
}
