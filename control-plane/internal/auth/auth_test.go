package auth

import (
	"strings"
	"testing"
	"time"
)

func testGate() *Gate {
	return NewGateFromSecrets("correct-horse-battery", []byte(strings.Repeat("k", 32)), Options{})
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Ada Lovelace":          "ada-lovelace",
		"  --Bob--  ":           "bob",
		"Ünïcödé":               "n-c-d",
		"":                      "",
		"!!!":                   "",
		strings.Repeat("a", 40): strings.Repeat("a", 32),
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCookieRoundTrip(t *testing.T) {
	g := testGate()
	c := g.Cookie("ada")
	owner, err := g.Verify(c.Value)
	if err != nil || owner != "ada" {
		t.Fatalf("verify: %q %v", owner, err)
	}
	// Tampering with the owner must break the signature.
	tampered := strings.Replace(c.Value, "ada|", "eve|", 1)
	if _, err := g.Verify(tampered); err == nil {
		t.Fatal("tampered cookie verified")
	}
	// A different secret must not verify.
	other := NewGateFromSecrets("x", []byte(strings.Repeat("z", 32)), Options{})
	if _, err := other.Verify(c.Value); err == nil {
		t.Fatal("cookie verified with wrong secret")
	}
	if _, err := g.Verify("garbage"); err == nil {
		t.Fatal("garbage verified")
	}
}

func TestPasswordAndRateLimit(t *testing.T) {
	g := testGate()
	if !g.CheckPassword("correct-horse-battery") || g.CheckPassword("wrong") {
		t.Fatal("password check broken")
	}
	ip := "203.0.113.9"
	for i := 0; i < 5; i++ {
		if !g.Allow(ip) {
			t.Fatalf("attempt %d denied", i)
		}
	}
	if g.Allow(ip) {
		t.Fatal("sixth attempt within a minute allowed")
	}
	// Refill is 5 per 60 s: after 13 s one token is back.
	g.rlMu.Lock()
	g.rlTokens[ip].last = time.Now().Add(-13 * time.Second)
	g.rlMu.Unlock()
	if !g.Allow(ip) {
		t.Fatal("bucket did not refill")
	}
}
