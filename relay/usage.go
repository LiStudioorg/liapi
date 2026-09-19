package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// TailBuffer keeps only the last max bytes written. Used to capture usage
// from the end of a JSON body without buffering the entire response.
type TailBuffer struct {
	max int
	buf []byte
}

func (t *TailBuffer) Write(p []byte) (int, error) {
	if len(p) >= t.max {
		t.buf = append(t.buf[:0], p[len(p)-t.max:]...)
		return len(p), nil
	}
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *TailBuffer) Bytes() []byte { return t.buf }

// ExtractUsage scans data for a "usage" object and returns
// (inTokens, outTokens). It works on both complete JSON and a tail slice
// (scan for `"usage"`, then brace-match from the following '{').
//
// Compatible with OpenAI keys (prompt_tokens/completion_tokens) and
// Anthropic keys (input_tokens/output_tokens).
func ExtractUsage(data []byte) (int, int) {
	if len(data) == 0 {
		return 0, 0
	}
	idx := bytes.Index(data, []byte(`"usage"`))
	if idx < 0 {
		return 0, 0
	}
	open := bytes.IndexByte(data[idx:], '{')
	if open < 0 {
		return 0, 0
	}
	start := idx + open
	depth := 0
	end := -1
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if depth == 0 && end > 0 {
			break
		}
	}
	if end < 0 || end-start > 1<<20 {
		return 0, 0
	}
	var u struct {
		PromptTokens     any `json:"prompt_tokens"`
		CompletionTokens any `json:"completion_tokens"`
		InputTokens      any `json:"input_tokens"`
		OutputTokens     any `json:"output_tokens"`
	}
	if err := json.Unmarshal(data[start:end], &u); err != nil {
		return 0, 0
	}
	in := toInt(u.PromptTokens)
	if in == 0 {
		in = toInt(u.InputTokens)
	}
	out := toInt(u.CompletionTokens)
	if out == 0 {
		out = toInt(u.OutputTokens)
	}
	return in, out
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
		return 0
	case string:
		var i int
		_, _ = fmt.Sscanf(n, "%d", &i)
		return i
	}
	return 0
}
