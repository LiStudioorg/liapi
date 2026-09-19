package stats

import (
	"strconv"
	"testing"
)

func TestLimiterWindow(t *testing.T) {
	l := NewLimiter(2)
	defer func() { _ = l }()
	for i := 0; i < 2; i++ {
		if !l.Allow("tok-a") {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if l.Allow("tok-a") {
		t.Fatal("3rd request should be denied")
	}
	if !l.Allow("tok-b") {
		t.Fatal("different token should be allowed")
	}
	l.SetLimit(0)
	if !l.Allow("tok-a") {
		t.Fatal("limit 0 should allow everything")
	}
}

func TestRingRecent(t *testing.T) {
	r := NewRing(16)
	for i := 0; i < 20; i++ {
		r.Add(Entry{Model: "m" + strconv.Itoa(i)})
	}
	recent := r.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("want 3, got %d", len(recent))
	}
	if recent[0].Model != "m19" || recent[2].Model != "m17" {
		t.Fatalf("unexpected order: %+v", recent)
	}
	if got := r.Recent(100); len(got) != 16 {
		t.Fatalf("want 16 (ring size), got %d", len(got))
	}
}
