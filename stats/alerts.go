package stats

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"liapi/config"
)

// Alerter fans out operational events (failover, unhealthy, quota) to
// configured webhook / Bark destinations. Pure stdlib, best-effort: failures
// are logged, never block the request path.
type Alerter struct {
	holder *config.Holder
	client *http.Client

	mu   sync.Mutex
	last map[string]time.Time // kind → last sent (dedup window)
}

func NewAlerter(holder *config.Holder, client *http.Client) *Alerter {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Alerter{
		holder: holder,
		client: client,
		last:   make(map[string]time.Time),
	}
}

type alertPayload struct {
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

// Notify sends an alert if cfg.Alerts enables this kind and the dedup
// window for kind+key has elapsed. Never blocks the caller.
func (a *Alerter) Notify(kind, key, title, message string) {
	if a == nil {
		return
	}
	cfg := a.holder.Get()
	enabled := false
	switch kind {
	case "failover":
		enabled = cfg.Alerts.OnFailover
	case "unhealthy", "recovered":
		enabled = cfg.Alerts.OnUnhealthy
	case "quota":
		enabled = cfg.Alerts.OnQuota
	default:
		return
	}
	if !enabled {
		return
	}
	if !a.shouldSend(kind+"|"+key, cfg.Alerts.MinAlertIntervalS) {
		return
	}
	p := alertPayload{
		Kind:    kind,
		Title:   title,
		Message: message,
		Time:    time.Now().Format(time.RFC3339),
	}
	body, err := json.Marshal(p)
	if err != nil {
		return
	}
	for _, u := range cfg.Alerts.Webhooks {
		a.post(u, body)
	}
	// Bark accepts {"title","body"} JSON on its device URL.
	barkBody, _ := json.Marshal(map[string]string{
		"title": title,
		"body":  message,
		"group": "liapi",
	})
	for _, u := range cfg.Alerts.Bark {
		a.post(u, barkBody)
	}
}

func (a *Alerter) shouldSend(key string, intervalS int) bool {
	if intervalS <= 0 {
		intervalS = 60
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if t, ok := a.last[key]; ok && now.Sub(t) < time.Duration(intervalS)*time.Second {
		return false
	}
	a.last[key] = now
	return true
}

func (a *Alerter) post(url string, body []byte) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("[alert] %s: %v", url, err)
		return
	}
	resp.Body.Close()
}
