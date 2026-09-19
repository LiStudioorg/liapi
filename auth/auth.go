package auth

import (
	"net/http"
	"strings"

	"liapi/common"
	"liapi/config"
)

// Client validates client tokens (the ones users send to /v1/*).
type Client struct {
	holder *config.Holder
}

func NewClient(holder *config.Holder) *Client {
	return &Client{holder: holder}
}

// Authenticate extracts the client token from the request, checking in order:
//  1. Authorization: Bearer <token>
//  2. x-api-key: <token>
//  3. ?token=<token>
func (c *Client) Authenticate(r *http.Request) string {
	if t := bearerFromHeader(r.Header.Get("Authorization")); t != "" {
		return t
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func (c *Client) Valid(token string) bool {
	if token == "" {
		return false
	}
	for _, t := range c.holder.Get().ClientTokens {
		if common.TokenEqual(t, token) {
			return true
		}
	}
	return false
}

// bearerFromHeader parses "Bearer <token>" (Bearer prefix case-insensitive).
// Malformed headers return "" -> caller treats as unauthorized.
func bearerFromHeader(h string) string {
	if h == "" {
		return ""
	}
	i := strings.IndexByte(h, ' ')
	if i <= 0 {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(h[:i]), "Bearer") {
		return ""
	}
	return strings.TrimSpace(h[i+1:])
}
