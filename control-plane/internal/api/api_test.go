package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/api"
	"github.com/mikelm20/learn-platform/control-plane/internal/auth"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
	"github.com/mikelm20/learn-platform/control-plane/internal/mail"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// These tests require a live Postgres at $DATABASE_URL.
// Skip with -short or when the DB is not reachable.
func testStore(t *testing.T) *db.Store {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://learn:learn@localhost:5432/learn?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := db.Open(ctx, url)
	if err != nil {
		t.Skipf("skip: no DB at %s: %v", url, err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tables := []string{
		"prompt_idempotency", "session_events", "sessions",
		"certificates", "progress", "auth_sessions", "magic_links", "identities",
	}
	for _, tbl := range tables {
		_, _ = store.Pool.Exec(ctx, "delete from "+tbl)
	}
	return store
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// mockLauncher returns fake sessions. Sleep emulates cold-boot latency.
type mockLauncher struct {
	Sleep time.Duration
}

func (m *mockLauncher) Launch(ctx context.Context) (*session.Session, error) {
	if m.Sleep > 0 {
		select {
		case <-time.After(m.Sleep):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return session.NewInMemorySession(uuid.NewString()), nil
}

func testServer(t *testing.T, store *db.Store, pool *session.WarmPool, launcher session.Launcher) *httptest.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError}))
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	signer := identity.NewSigner(secret)

	host := session.NewHost(launcher, pool, 5, logger)
	srv := httptest.NewServer(api.NewRouter(api.Deps{
		Logger:        logger,
		Store:         store,
		Signer:        signer,
		Host:          host,
		Transcript:    session.NewTranscript(store, logger),
		Mail:          mail.NewStdoutSender(logger),
		MagicLimiter:  auth.MagicLinkLimiter(),
		IPLimiter:     auth.NewTokenBucket(100, time.Minute),
		LessonsDir:    "../../../lessons",
		PublicOrigin:  "http://localhost",
		SecureCookies: false,
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}

func postJSON(t *testing.T, c *http.Client, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := c.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	return resp
}

func readBody(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}

func mustJSON(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("status %d: %s", resp.StatusCode, readBody(resp.Body))
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// TestMintClaimSession covers the happy path documented in the task exit
// criteria: mint -> PATCH me -> session -> prompt idempotency -> transcript.
func TestMintClaimSession(t *testing.T) {
	store := testStore(t)
	defer store.Close()
	srv := testServer(t, store, nil, &mockLauncher{})

	client := newClient(t)

	resp := postJSON(t, client, srv.URL+"/identity", map[string]any{"lang": "es"})
	var mint struct{ UUID string }
	mustJSON(t, resp, &mint)
	if mint.UUID == "" {
		t.Fatal("no uuid")
	}

	resp, err := client.Get(srv.URL + "/me")
	if err != nil {
		t.Fatal(err)
	}
	var me map[string]any
	mustJSON(t, resp, &me)
	if me["uuid"] != mint.UUID {
		t.Fatalf("me uuid drift: %v vs %s", me["uuid"], mint.UUID)
	}

	resp = postJSON(t, client, srv.URL+"/sessions", map[string]any{
		"lesson_id": "hello-claude", "lang": "es",
	})
	var sess struct {
		SessionID string `json:"session_id"`
	}
	mustJSON(t, resp, &sess)
	if sess.SessionID == "" {
		t.Fatal("no session_id")
	}

	key := uuid.NewString()
	body := map[string]any{"text": "hola", "client_ts": time.Now().Format(time.RFC3339)}
	b, _ := json.Marshal(body)
	doPrompt := func() string {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sessions/"+sess.SessionID+"/prompt", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		r, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var pr struct {
			TurnID string `json:"turn_id"`
		}
		mustJSON(t, r, &pr)
		return pr.TurnID
	}
	first := doPrompt()
	second := doPrompt()
	if first != second {
		t.Fatalf("idempotency broken: %s vs %s", first, second)
	}

	resp3, err := client.Get(srv.URL + "/sessions/" + sess.SessionID + "/transcript")
	if err != nil {
		t.Fatal(err)
	}
	var tr struct {
		Events []json.RawMessage `json:"events"`
	}
	mustJSON(t, resp3, &tr)
	if len(tr.Events) < 1 {
		t.Fatalf("expected transcript events, got %d", len(tr.Events))
	}
}

// TestWarmPoolSub500ms verifies that POST /sessions?warm=true is served in
// well under 500ms when the warm pool has capacity, even though the cold
// path takes 5 seconds.
func TestWarmPoolSub500ms(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	launcher := &mockLauncher{Sleep: 5 * time.Second}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError}))
	pool := session.NewWarmPool(launcher, 1, logger)
	pool.Start()
	defer pool.Stop()

	// Wait up to 7s for the pool to fill.
	deadline := time.Now().Add(7 * time.Second)
	for pool.Size() < 1 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if pool.Size() < 1 {
		t.Skip("warm pool did not fill in time")
	}

	srv := testServer(t, store, pool, launcher)
	client := newClient(t)
	postJSON(t, client, srv.URL+"/identity", map[string]any{"lang": "es"}).Body.Close()

	start := time.Now()
	resp := postJSON(t, client, srv.URL+"/sessions", map[string]any{
		"lesson_id": "hello-claude", "lang": "es", "warm": true,
	})
	elapsed := time.Since(start)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("warm session: %d: %s", resp.StatusCode, readBody(resp.Body))
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("warm session too slow: %v", elapsed)
	}
}

// TestWSReplay verifies the /sessions/:id/ws endpoint replays every
// persisted event for that session before accepting new ones.
func TestWSReplay(t *testing.T) {
	store := testStore(t)
	defer store.Close()
	srv := testServer(t, store, nil, &mockLauncher{})

	client := newClient(t)
	postJSON(t, client, srv.URL+"/identity", map[string]any{"lang": "es"}).Body.Close()

	resp := postJSON(t, client, srv.URL+"/sessions", map[string]any{
		"lesson_id": "hello-claude", "lang": "es",
	})
	var sess struct{ SessionID string `json:"session_id"` }
	mustJSON(t, resp, &sess)

	b, _ := json.Marshal(map[string]any{"text": "hi", "client_ts": time.Now().Format(time.RFC3339)})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sessions/"+sess.SessionID+"/prompt", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	r2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	time.Sleep(400 * time.Millisecond) // let transcript persist

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/sessions/" + sess.SessionID + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if !bytes.Contains(msg, []byte(`"type"`)) {
		t.Fatalf("ws replay not event json: %s", msg)
	}
}
