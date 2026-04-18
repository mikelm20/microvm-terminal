package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

// Preview reverse-proxies browser requests to a port the learner opened inside
// their VM. Route shape: GET /sessions/{id}/preview/{port}/<rest of path>.
//
// MVP notes:
//   - Same-origin URL, auth cookie covers it via the existing middleware.
//   - Go's httputil.ReverseProxy handles WebSocket upgrades transparently (>=1.20),
//     so dev servers with HMR work out of the box.
//   - Relative URLs in the learner's HTML resolve against the prefixed path, so
//     `<script src="/app.js">` will 404. For Phase 2 we move previews to a
//     per-session subdomain (`<sid>.preview.learn.example.com`) which sidesteps
//     this entirely. The current approach is fine for simple static demos.
func (h *sessionHandler) Preview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	portStr := r.PathValue("port")
	s, ok := h.mgr.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "invalid port", http.StatusBadRequest)
		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   s.IP.String() + ":" + strconv.Itoa(port),
	}

	prefix := "/sessions/" + id + "/preview/" + portStr
	rp := httputil.NewSingleHostReverseProxy(target)
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.URL.RawPath = ""
		// Override Host so downstream HTTP vhost checks don't trip on our domain.
		req.Host = target.Host
	}
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.logger.Debug("preview proxy", "err", err, "session", id, "port", port)
		http.Error(w, "upstream not reachable: "+err.Error(), http.StatusBadGateway)
	}
	rp.ServeHTTP(w, r)
}
