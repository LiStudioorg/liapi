package relay

import (
	"bytes"
	"testing"
)

func TestExtractUsageOpenAI(t *testing.T) {
	data := []byte(`{"id":"x","usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`)
	in, out := ExtractUsage(data)
	if in != 5 || out != 7 {
		t.Fatalf("got in=%d out=%d, want 5/7", in, out)
	}
}

func TestExtractUsageAnthropic(t *testing.T) {
	data := []byte(`{"type":"message_start","message":{"usage":{"input_tokens":9,"output_tokens":3}}}`)
	in, out := ExtractUsage(data)
	if in != 9 || out != 3 {
		t.Fatalf("got in=%d out=%d, want 9/3", in, out)
	}
}

func TestExtractUsageFromTail(t *testing.T) {
	// Simulate only a tail of a big response arriving (usage key kept).
	full := `{"choices":[{"message":{"content":"x"}}],"usage":{"prompt_tokens":11,"completion_tokens":22}}`
	in, out := ExtractUsage([]byte(full[len(full)-60:]))
	if in != 11 || out != 22 {
		t.Fatalf("tail extraction got in=%d out=%d, want 11/22", in, out)
	}
}

func TestExtractUsageMissing(t *testing.T) {
	in, out := ExtractUsage([]byte(`{"ok":true}`))
	if in != 0 || out != 0 {
		t.Fatalf("no usage should be 0/0, got %d/%d", in, out)
	}
}

func TestTailBuffer(t *testing.T) {
	tb := &TailBuffer{max: 8}
	_, _ = tb.Write([]byte("01234567890abcdef"))
	got := string(tb.Bytes())
	if got != "90abcdef" {
		t.Fatalf("unexpected tail: %q", got)
	}
}

func TestDetectStream(t *testing.T) {
	if !DetectStream([]byte(`{"stream":true}`)) {
		t.Fatal("stream:true should be detected")
	}
	if DetectStream([]byte(`{"stream":false}`)) {
		t.Fatal("stream:false should not trigger")
	}
	if DetectStream([]byte(`{"messages":[]}`)) {
		t.Fatal("no stream field should not trigger")
	}
	if DetectStream([]byte(`not json`)) {
		t.Fatal("bad json should not trigger")
	}
}

func TestRelativePath(t *testing.T) {
	cases := map[string]string{
		"/v1/chat/completions": "/chat/completions",
		"/v1/messages":         "/messages",
		"/v1":                  "/",
		"/custom/path":         "/custom/path",
	}
	for in, want := range cases {
		if got := relativePath(in); got != want {
			t.Fatalf("relativePath(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestPrepareBody(t *testing.T) {
	// No change needed: original bytes preserved.
	in := []byte(`{"model":"gpt-4o","stream":false}`)
	if got := prepareBody(in, "gpt-4o", "gpt-4o", false, true); !bytes.Equal(got, in) {
		t.Fatalf("no-rewrite path should return original body")
	}
	// Alias rewrite.
	out := prepareBody([]byte(`{"model":"gpt-4o","stream":true}`), "gpt-4o", "openai/gpt-4o", true, false)
	if !bytes.Contains(out, []byte(`"openai/gpt-4o"`)) {
		t.Fatalf("model not rewritten: %s", out)
	}
	// inject usage.
	out = prepareBody([]byte(`{"model":"gpt-4o","stream":true}`), "gpt-4o", "gpt-4o", true, true)
	if !bytes.Contains(out, []byte(`"include_usage"`)) {
		t.Fatalf("include_usage not injected: %s", out)
	}
}
