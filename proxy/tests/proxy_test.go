// Integration tests for the claude-proxy. A fake Anthropic upstream runs in
// httptest, the proxy points at it, and requests assert: auth, quota, audit,
// streaming frames preserved, rotation drains cleanly.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mikelm20/learn-platform/proxy/internal/audit"
	"github.com/mikelm20/learn-platform/proxy/internal/keys"
	"github.com/mikelm20/learn-platform/proxy/internal/quota"
	"github.com/mikelm20/learn-platform/proxy/internal/server"
)

func newFakeAnthropic(t *testing.T, body string, status int, isStream bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got == "" {
			t.Errorf("upstream did not receive x-api-key")
		}
		if strings.Contains(r.Header.Get("Authorization"), "Bearer") {
			t.Errorf("upstream saw the learner bearer token; must be stripped")
		}
		if isStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(status)
			flusher, _ := w.(http.Flusher)
			frames := []string{
				`event: message_start` + "\n" +
					`data: {"type":"message_start","message":{"id":"m1","usage":{"input_tokens":42,"output_tokens":0}}}` + "\n\n",
				`event: content_block_delta` + "\n" +
					`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}` + "\n\n",
				`event: message_delta` + "\n" +
					`data: {"type":"message_delta","usage":{"input_tokens":42,"output_tokens":73}}` + "\n\n",
			}
			for _, f := range frames {
				_, _ = io.WriteString(w, f)
				if flusher != nil {
					flusher.Flush()
				}
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

func newProxy(t *testing.T, upstream string) (*httptest.Server, *audit.InMemorySink, *quota.Manager, *keys.Pool) {
	t.Helper()
	sink := audit.NewInMemorySink()
	pool := keys.New([]keys.Key{{ID: "k1", Workspace: "learn-dev", Secret: "sk-ant-fake"}})
	qm := quota.New(quota.Limits{
		MaxTokensIn:          10_000,
		MaxTokensOut:         20_000,
		MaxRequests:          5,
		MaxRequestsPerMinute: 5,
	})
	srv, err := server.New(server.Config{
		Upstream:   upstream,
		Pool:       pool,
		Quota:      qm,
		Audit:      sink,
		AdminToken: "admin-secret",
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return httptest.NewServer(srv.Handler()), sink, qm, pool
}

func TestMissingBearerReturns401(t *testing.T) {
	up := newFakeAnthropic(t, `{"ok":true}`, 200, false)
	defer up.Close()
	proxy, _, _, _ := newProxy(t, up.URL)
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(`{}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
	var er map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&er)
	if er["code"] != "auth_required" {
		t.Fatalf("want code=auth_required, got %v", er["code"])
	}
}

func TestUnknownSessionReturns401(t *testing.T) {
	up := newFakeAnthropic(t, `{"ok":true}`, 200, false)
	defer up.Close()
	proxy, _, _, _ := newProxy(t, up.URL)
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer sess_notminted")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 for unknown token, got %d", resp.StatusCode)
	}
}

func TestHappyPathStreamingAndAudit(t *testing.T) {
	up := newFakeAnthropic(t, "", 200, true)
	defer up.Close()
	proxy, sink, qm, pool := newProxy(t, up.URL)
	defer proxy.Close()

	token, err := pool.MintSessionToken("s1")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"input_tokens":42`) || !strings.Contains(buf.String(), `"output_tokens":73`) {
		t.Fatalf("stream body did not preserve upstream frames: %s", buf.String())
	}
	// Wait briefly for the async audit write to land.
	if !waitFor(t, 2*time.Second, func() bool { return sink.Len() >= 1 }) {
		t.Fatalf("audit sink never received a record, len=%d", sink.Len())
	}
	rec := sink.All()[0]
	if rec.RequestTokens != 42 || rec.ResponseTokens != 73 {
		t.Fatalf("audit usage wrong: in=%d out=%d", rec.RequestTokens, rec.ResponseTokens)
	}
	if rec.Status != 200 {
		t.Fatalf("audit status wrong: %d", rec.Status)
	}
	if u := qm.Usage(token); u.TokensIn != 42 || u.TokensOut != 73 {
		t.Fatalf("quota not committed: %+v", u)
	}
}

func TestRequestsPerMinuteLimit(t *testing.T) {
	up := newFakeAnthropic(t, `{"usage":{"input_tokens":1,"output_tokens":1}}`, 200, false)
	defer up.Close()
	proxy, _, _, pool := newProxy(t, up.URL)
	defer proxy.Close()

	token, _ := pool.MintSessionToken("s1")
	var got429 bool
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
			if resp.Header.Get("Retry-After") == "" {
				t.Fatalf("429 response missing Retry-After")
			}
			break
		}
	}
	if !got429 {
		t.Fatalf("expected 429 within 10 consecutive calls with RPM=5")
	}
}

func TestTotalRequestsLimit(t *testing.T) {
	up := newFakeAnthropic(t, `{"usage":{"input_tokens":1,"output_tokens":1}}`, 200, false)
	defer up.Close()
	proxy, _, qm, pool := newProxy(t, up.URL)
	defer proxy.Close()

	// Squeeze the RPM out of the picture by advancing the clock between
	// calls so only the cumulative request cap can bite.
	base := time.Now()
	qm.SetNow(func() time.Time { return base })
	token, _ := pool.MintSessionToken("s1")

	var statuses []int
	for i := 0; i < 7; i++ {
		// Bump the clock by >60s each iteration so the rate limiter resets.
		qm.SetNow(func() time.Time { return base.Add(time.Duration(i+1) * 2 * time.Minute) })
		req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		statuses = append(statuses, resp.StatusCode)
	}
	// With MaxRequests=5 we should see five 2xx then 429s.
	var ok int
	var limited int
	for _, s := range statuses {
		if s == 200 {
			ok++
		}
		if s == http.StatusTooManyRequests {
			limited++
		}
	}
	if ok != 5 || limited < 1 {
		t.Fatalf("expected 5 ok + >=1 limited, got ok=%d limited=%d statuses=%v", ok, limited, statuses)
	}
}

func TestAdminRotateDrainsThenSwaps(t *testing.T) {
	up := newFakeAnthropic(t, `{"usage":{"input_tokens":1,"output_tokens":1}}`, 200, false)
	defer up.Close()
	proxy, _, _, pool := newProxy(t, up.URL)
	defer proxy.Close()

	old, _ := pool.MintSessionToken("s1")

	// Run a few concurrent requests to create in-flight work.
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer "+old)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}()
	}

	newKeys := []keys.Key{{ID: "k2", Workspace: "learn-dev", Secret: "sk-ant-rotated"}}
	body, _ := json.Marshal(newKeys)
	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/admin/rotate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("rotate failed: %d", resp.StatusCode)
	}
	wg.Wait()

	// After rotation, new mint should succeed with the new key.
	next, err := pool.MintSessionToken("s2")
	if err != nil {
		t.Fatalf("mint after rotate: %v", err)
	}
	// And the secret resolves to the rotated value.
	secret, _, err := pool.SecretForSession(next)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "sk-ant-rotated" {
		t.Fatalf("post-rotate secret wrong: %s", secret)
	}
	// Old token mapping survives but its key is gone; lookups now fail with
	// ErrKeyRevoked, which the proxy translates to 503.
	if _, _, err := pool.SecretForSession(old); err == nil {
		t.Fatalf("old session should fail after rotation")
	}
}

func TestAdminRotateRequiresAdminToken(t *testing.T) {
	up := newFakeAnthropic(t, `{}`, 200, false)
	defer up.Close()
	proxy, _, _, _ := newProxy(t, up.URL)
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/admin/rotate", strings.NewReader(`[]`))
	// no auth
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 without admin token, got %d", resp.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	up := newFakeAnthropic(t, `{}`, 200, false)
	defer up.Close()
	proxy, _, _, _ := newProxy(t, up.URL)
	defer proxy.Close()

	resp, err := http.Get(proxy.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthz failed: %d", resp.StatusCode)
	}
}

func TestErrorShapeMatchesApiError(t *testing.T) {
	up := newFakeAnthropic(t, `{}`, 200, false)
	defer up.Close()
	proxy, _, _, _ := newProxy(t, up.URL)
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/messages", strings.NewReader(`{}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var er struct {
		Code              string `json:"code"`
		Message           string `json:"message"`
		RequestID         string `json:"request_id"`
		RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&er)
	if er.Code == "" || er.Message == "" || er.RequestID == "" {
		t.Fatalf("ApiError shape incomplete: %+v", er)
	}
}

func waitFor(t *testing.T, d time.Duration, fn func() bool) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		if fn() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
		}
	}
}

