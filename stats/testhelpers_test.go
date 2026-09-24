package stats

// Test helpers shared across the stats package tests.

import (
	"bytes"
	"net/http"

	"liapi/config"
)

// ConfigForTest builds a minimal holder with the given health state file.
func ConfigForTest(healthStateFile string) *config.Holder {
	c := &config.Config{HealthStateFile: healthStateFile}
	c.SetDefaults()
	c.HealthStateFile = healthStateFile
	return config.NewHolder(c)
}

// fakeRW is a minimal http.ResponseWriter capturing the body for
// WritePrometheus tests.
type fakeRW struct {
	hdr  map[string][]string
	body bytes.Buffer
	code int
}

func (f *fakeRW) Header() http.Header {
	if f.hdr == nil {
		f.hdr = map[string][]string{}
	}
	return f.hdr
}

func (f *fakeRW) Write(p []byte) (int, error) { return f.body.Write(p) }

func (f *fakeRW) WriteHeader(code int) { f.code = code }
