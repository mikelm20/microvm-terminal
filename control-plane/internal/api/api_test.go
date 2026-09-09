package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/mikelm20/microvm-terminal/control-plane/internal/api"
	"github.com/mikelm20/microvm-terminal/control-plane/internal/auth"
	"github.com/mikelm20/microvm-terminal/control-plane/internal/db"
	"github.com/mikelm20/microvm-terminal/control-plane/internal/session"
)

const testPassword = "open-sesame-8"

// memStore is an in-memory SessionStore so the handler tests need no
// Postgres. TestWithPostgres exercises the real one when DATABASE_URL points
// at a database.
type memStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*db.Session
}

func newMemStore() *memStore { return &memStore{rows: map[uuid.UUID]*db.Session{}} }

func (m *memStore) InsertSession(_ context.Context, s db.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.CreatedAt = time.Now()
	m.rows[s.ID] = &s
	return nil
}

func (m *memStore) GetSession(_ context.Context, id uuid.UUID) (*db.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rows[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, db.ErrNotFound
}

func (m *memStore) MarkSessionReaped(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rows[id]; ok && !r.ReapedAt.Valid {
		r.ReapedAt.Valid = true
		r.ReapedAt.Time = time.Now()
	}
	return nil
}

func (m *memStore) TouchSessionAttached(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rows[id]; ok {
		r.LastAttachedAt.Valid = true
		r.LastAttachedAt.Time = time.Now()
	}
	return nil
}

// fakeProc stands in for Firecracker: whatever the client writes lands in
// stdinBuf; whatever the test writes to stdoutW shows up on the serial tee.
type fakeProc struct {
	stdinMu  sync.Mutex
	stdinBuf bytes.Buffer
	stdoutR  *io.PipeReader
	stdoutW  *io.PipeWriter
}

func newFakeProc() *fakeProc {
	r, w := io.Pipe()
	return &fakeProc{stdoutR: r, stdoutW: w}
}

func (p *fakeProc) Write(b []byte) (int, error) {
	p.stdinMu.Lock()
	defer p.stdinMu.Unlock()
	return p.stdinBuf.Write(b)
}
func (p *fakeProc) Stdin() io.Writer  { return p }
func (p *fakeProc) Stdout() io.Reader { return p.stdoutR }
func (p *fakeProc) Stop() error       { return p.stdoutW.Close() }
func (p *fakeProc) Wait() error       { return nil }
func (p *fakeProc) VmDir() string     { return "" }
func (p *fakeProc) stdinString() string {
	p.stdinMu.Lock()
	defer p.stdinMu.Unlock()
	return p.stdinBuf.String()
}

// consoleLauncher returns sessions backed by fakeProc so the PTY WebSocket
// has something to bridge.
type consoleLauncher struct {
	mu    sync.Mutex
	procs map[string]*fakeProc
	sleep time.Duration
}

func (l *consoleLauncher) Launch(ctx context.Context) (*session.Session, error) {
	if l.sleep > 0 {
		select {
		case <-time.After(l.sleep):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s := session.NewInMemorySession(uuid.NewString())
	p := newFakeProc()
	s.Process = p
	s.Serial = session.NewSerialTee(p.Stdout(), 64*1024)
	l.mu.Lock()
	if l.procs == nil {
		l.procs = map[string]*fakeProc{}
	}
	l.procs[s.ID] = p
	l.mu.Unlock()
	return s, nil
}

func (l *consoleLauncher) proc(id string) *fakeProc {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.procs[id]
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func testServer(t *testing.T, store api.SessionStore, launcher session.Launcher, maxLive int) (*httptest.Server, *session.Host) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError}))
	gate := auth.NewGateFromSecrets(testPassword, bytes.Repeat([]byte{7}, 32), auth.Options{})
	host := session.NewHost(launcher, nil, maxLive, logger)
	srv := httptest.NewServer(api.NewRouter(api.Deps{
		Logger:       logger,
		Store:        store,
		Gate:         gate,
		Host:         host,
		PublicOrigin: "http://localhost",
	}))
	t.Cleanup(srv.Close)
	return srv, host
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func login(t *testing.T, c *http.Client, base, name, password string) *http.Response {
	t.Helper()
	form := url.Values{"name": {name}, "password": {password}}
	resp, err := c.PostForm(base+"/login", form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
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

func mustJSON(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestLoginFlow(t *testing.T) {
	srv, _ := testServer(t, newMemStore(), &consoleLauncher{}, 2)
	c := newClient(t)

	resp, err := c.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "<form") {
		t.Fatalf("login page: %d", resp.StatusCode)
	}

	if r := login(t, c, srv.URL, "Ada", "nope"); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", r.StatusCode)
	}
	if r := postJSON(t, c, srv.URL+"/sessions", map[string]any{}); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sessions without cookie: %d", r.StatusCode)
	}
	if r := login(t, c, srv.URL, "!!!", testPassword); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty slug: %d", r.StatusCode)
	}
	r := login(t, c, srv.URL, "Ada Lovelace", testPassword)
	if r.StatusCode != http.StatusSeeOther || r.Header.Get("Location") != "/terminal" {
		t.Fatalf("login: %d -> %q", r.StatusCode, r.Header.Get("Location"))
	}
	resp, err = c.Get(srv.URL + "/terminal")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "ada-lovelace") {
		t.Fatalf("terminal page: %d", resp.StatusCode)
	}

	// JSON login for API clients.
	jr := postJSON(t, c, srv.URL+"/login", map[string]string{"name": "bot", "password": testPassword})
	jr.Body.Close()
	if jr.StatusCode != http.StatusNoContent {
		t.Fatalf("json login: %d", jr.StatusCode)
	}
}

func TestSessionLifecycle(t *testing.T) {
	store := newMemStore()
	srv, host := testServer(t, store, &consoleLauncher{}, 1)
	ada := newClient(t)
	login(t, ada, srv.URL, "ada", testPassword)
	eve := newClient(t)
	login(t, eve, srv.URL, "eve", testPassword)

	r := ada.Get
	resp, err := r(srv.URL + "/sessions/current")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("current before boot: %d", resp.StatusCode)
	}

	var created api.CreateSessionResponse
	mustJSON(t, postJSON(t, ada, srv.URL+"/sessions", map[string]any{}), &created)
	if created.SessionID == "" || !strings.HasSuffix(created.PTYWSURL, "/sessions/"+created.SessionID+"/pty") {
		t.Fatalf("create: %+v", created)
	}

	var cur api.SessionInfo
	resp, _ = ada.Get(srv.URL + "/sessions/current")
	mustJSON(t, resp, &cur)
	if cur.SessionID != created.SessionID || !cur.Alive || cur.Owner != "ada" {
		t.Fatalf("current: %+v", cur)
	}

	// Capacity is one: a second boot is refused with a retry hint.
	resp = postJSON(t, eve, srv.URL+"/sessions", map[string]any{})
	var apiErr api.APIError
	json.NewDecoder(resp.Body).Decode(&apiErr)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || apiErr.Code != api.ErrCapacityFull || apiErr.RetryAfterSeconds == nil {
		t.Fatalf("capacity: %d %+v", resp.StatusCode, apiErr)
	}

	// Eve cannot see or delete Ada's VM.
	resp, _ = eve.Get(srv.URL + "/sessions/" + created.SessionID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-owner describe: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/sessions/"+created.SessionID, nil)
	resp, _ = eve.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-owner delete: %d", resp.StatusCode)
	}
	if host.Count() != 1 {
		t.Fatal("eve's delete removed ada's session")
	}

	// Ada deletes; the row is stamped reaped and Describe reports it dead.
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/sessions/"+created.SessionID, nil)
	resp, _ = ada.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 || host.Count() != 0 {
		t.Fatalf("delete: %d live=%d", resp.StatusCode, host.Count())
	}
	var gone api.SessionInfo
	resp, _ = ada.Get(srv.URL + "/sessions/" + created.SessionID)
	mustJSON(t, resp, &gone)
	if gone.Alive {
		t.Fatal("deleted session reported alive")
	}
}

func TestPTYBridgeAndResize(t *testing.T) {
	launcher := &consoleLauncher{}
	srv, host := testServer(t, newMemStore(), launcher, 2)
	c := newClient(t)
	login(t, c, srv.URL, "ada", testPassword)

	var created api.CreateSessionResponse
	mustJSON(t, postJSON(t, c, srv.URL+"/sessions", map[string]any{}), &created)
	proc := launcher.proc(created.SessionID)
	live, _ := host.Get(created.SessionID)

	// Bytes written by the "VM" before the client attaches are replayed.
	proc.stdoutW.Write([]byte("boot log\r\n"))
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/sessions/" + created.SessionID + "/pty"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPClient: c})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	typ, msg, err := conn.Read(ctx)
	if err != nil || typ != websocket.MessageBinary || string(msg) != "boot log\r\n" {
		t.Fatalf("replay: %v %v %q", typ, err, msg)
	}
	deadline := time.Now().Add(time.Second)
	for live.Attached() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if live.Attached() != 1 {
		t.Fatalf("attached = %d", live.Attached())
	}

	// Live output streams through.
	proc.stdoutW.Write([]byte("$ "))
	_, msg, err = conn.Read(ctx)
	if err != nil || string(msg) != "$ " {
		t.Fatalf("stream: %v %q", err, msg)
	}

	// Keystrokes reach the serial stdin; resize lands on the session.
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("ls\r")); err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","cols":132,"rows":43}`)); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cols, rows := live.WindowSize()
		if proc.stdinString() == "ls\r" && cols == 132 && rows == 43 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if cols, rows := live.WindowSize(); proc.stdinString() != "ls\r" || cols != 132 || rows != 43 {
		t.Fatalf("stdin=%q size=%dx%d", proc.stdinString(), cols, rows)
	}

	// Destroying the VM closes the socket with the "vm exited" reason.
	if err := host.Destroy(created.SessionID); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.Read(ctx)
	var ce websocket.CloseError
	if err == nil || !asCloseError(err, &ce) || ce.Reason != "vm exited" {
		t.Fatalf("expected vm exited close, got %v", err)
	}
	deadline = time.Now().Add(time.Second)
	for live.Attached() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if live.Attached() != 0 {
		t.Fatal("client still counted as attached after close")
	}
}

func asCloseError(err error, ce *websocket.CloseError) bool {
	return errors.As(err, ce)
}

// TestWithPostgres runs the boot + describe flow against a real database
// when DATABASE_URL is reachable; skipped otherwise.
func TestWithPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://microvm:microvm@localhost:5432/microvm?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	store, err := db.Open(ctx, url)
	if err != nil {
		t.Skipf("skip: no DB at %s: %v", url, err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	srv, _ := testServer(t, store, &consoleLauncher{}, 2)
	c := newClient(t)
	login(t, c, srv.URL, "pg", testPassword)
	var created api.CreateSessionResponse
	mustJSON(t, postJSON(t, c, srv.URL+"/sessions", map[string]any{}), &created)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/sessions/"+created.SessionID, nil)
	resp, _ := c.Do(req)
	resp.Body.Close()
	row, err := store.GetSession(ctx, uuid.MustParse(created.SessionID))
	if err != nil || row.Owner != "pg" {
		t.Fatalf("row: %+v %v", row, err)
	}
}
