package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type ctxKey int

const requestIDKey ctxKey = 1

// RequestIDHeader is echoed to clients and forwarded upstream.
const RequestIDHeader = "X-Request-ID"

// withRequestID attaches rid to the request context and echoes the response
// header. If the client did not send one, a random id is generated.
func withRequestID(w http.ResponseWriter, r *http.Request) *http.Request {
	rid := r.Header.Get(RequestIDHeader)
	if rid == "" || len(rid) > 128 {
		rid = newRequestID()
	}
	w.Header().Set(RequestIDHeader, rid)
	return r.WithContext(context.WithValue(r.Context(), requestIDKey, rid))
}

func requestIDFrom(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey).(string); ok {
		return v
	}
	return r.Header.Get(RequestIDHeader)
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}
