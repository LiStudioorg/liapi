package auth

import (
	"net/http"
	"strings"

	"liapi/common"
	"liapi/config"
)

// Identity is the authenticated caller: either a registered device (hash
// lookup) or a legacy plaintext client token.
type Identity struct {
	// Token is the raw credential (never log it; use Mask for logs).
	Token string
	// DeviceID is set only when matched via devices[] (hash path).
	DeviceID string
	Device   *config.Device
	// Legacy is true when authenticated via client_tokens (plaintext list).
	Legacy bool
}

// Mask returns a log-safe representation.
func (id *Identity) Mask() string {
	if id == nil {
		return ""
	}
	return common.MaskToken(id.Token)
}

// Client validates client tokens (the ones users send to /v1/*).
// Lookup order:
//  1. devices[] — SHA-256 hash match (preferred; tokens stored hashed).
//  2. client_tokens[] — legacy plaintext list (constant-time compare).
type Client struct {
	holder *config.Holder
}

func NewClient(holder *config.Holder) *Client {
	return &Client{holder: holder}
}

// Identify extracts and validates the credential. Returns nil when invalid.
func (c *Client) Identify(r *http.Request) *Identity {
	token := ExtractToken(r)
	if token == "" {
		return nil
	}
	return c.identifyToken(token)
}

func (c *Client) identifyToken(token string) *Identity {
	cfg := c.holder.Get()
	// 1. Devices (hashed).
	if len(cfg.Devices) > 0 {
		for i := range cfg.Devices {
			d := &cfg.Devices[i]
			if d.Disabled {
				continue
			}
			if common.HashEqual(token, d.TokenHash) {
				// Copy the device so later config swaps don't race the caller.
				cp := *d
				return &Identity{Token: token, DeviceID: d.ID, Device: &cp}
			}
		}
	}
	// 2. Legacy plaintext list.
	for _, t := range cfg.ClientTokens {
		if common.TokenEqual(t, token) {
			return &Identity{Token: token, Legacy: true}
		}
	}
	return nil
}

// Authenticate extracts the client token from the request, checking in order:
//  1. Authorization: Bearer <token>
//  2. x-api-key: <token>
//  3. ?token=<token>
func (c *Client) Authenticate(r *http.Request) string {
	return ExtractToken(r)
}

// ExtractToken is the header/query parsing half of Authenticate.
func ExtractToken(r *http.Request) string {
	if t := bearerFromHeader(r.Header.Get("Authorization")); t != "" {
		return t
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

// Valid reports whether the token is accepted (device hash or legacy list).
func (c *Client) Valid(token string) bool {
	return c.identifyToken(token) != nil
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
