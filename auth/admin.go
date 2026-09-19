package auth

import (
	"net/http"

	"liapi/common"
	"liapi/config"
)

// Admin validates the separate admin token used for /admin/api/*.
type Admin struct {
	holder *config.Holder
}

func NewAdmin(holder *config.Holder) *Admin {
	return &Admin{holder: holder}
}

func (a *Admin) Authenticate(r *http.Request) bool {
	cfg := a.holder.Get()
	if cfg.AdminToken == "" {
		return false
	}
	t := bearerFromHeader(r.Header.Get("Authorization"))
	if t == "" {
		t = r.Header.Get("x-admin-token")
	}
	if t == "" {
		return false
	}
	return common.TokenEqual(cfg.AdminToken, t)
}
