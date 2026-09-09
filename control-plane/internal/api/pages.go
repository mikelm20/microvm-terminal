package api

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/mikelm20/microvm-terminal/control-plane/internal/auth"
	"github.com/mikelm20/microvm-terminal/control-plane/internal/web"
)

type pagesHandler struct {
	deps      Deps
	loginTmpl *template.Template
	termTmpl  *template.Template
}

func newPagesHandler(deps Deps) *pagesHandler {
	return &pagesHandler{
		deps:      deps,
		loginTmpl: template.Must(template.New("login").Parse(web.Login)),
		termTmpl:  template.Must(template.New("terminal").Parse(web.Terminal)),
	}
}

type loginView struct {
	Name  string
	Error string
}

// Login serves the form, or sends a logged-in caller straight to the terminal.
func (h *pagesHandler) Login(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.deps.Gate.OwnerFromRequest(r); ok {
		http.Redirect(w, r, "/terminal", http.StatusSeeOther)
		return
	}
	h.render(w, h.loginTmpl, loginView{})
}

// SubmitLogin accepts an HTML form (name, password) or a JSON body with the
// same fields. Form callers are redirected to /terminal; JSON callers get 204.
func (h *pagesHandler) SubmitLogin(w http.ResponseWriter, r *http.Request) {
	var name, password string
	wantsJSON := strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
	if wantsJSON {
		var body struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := decodeJSON(r, &body); err != nil {
			apiError(w, r, http.StatusBadRequest, ErrBadRequest, "invalid json")
			return
		}
		name, password = body.Name, body.Password
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		name, password = r.FormValue("name"), r.FormValue("password")
	}

	fail := func(status int, code, msg string) {
		if wantsJSON {
			apiError(w, r, status, code, msg)
			return
		}
		w.WriteHeader(status)
		h.render(w, h.loginTmpl, loginView{Name: name, Error: msg})
	}

	if !h.deps.Gate.Allow(auth.ClientIP(r)) {
		fail(http.StatusTooManyRequests, ErrRateLimited, "Too many attempts. Wait a minute and try again.")
		return
	}
	owner := auth.Slugify(name)
	if owner == "" {
		fail(http.StatusBadRequest, ErrBadRequest, "Pick a name with at least one letter or digit.")
		return
	}
	if !h.deps.Gate.CheckPassword(password) {
		// Equalise timing with the happy path a little.
		time.Sleep(150 * time.Millisecond)
		fail(http.StatusUnauthorized, ErrAuthInvalid, "Wrong password.")
		return
	}
	http.SetCookie(w, h.deps.Gate.Cookie(owner))
	h.deps.Logger.Info("login", "owner", owner, "ip", auth.ClientIP(r))
	if wantsJSON {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/terminal", http.StatusSeeOther)
}

func (h *pagesHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, h.deps.Gate.ClearCookie())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *pagesHandler) Terminal(w http.ResponseWriter, r *http.Request) {
	owner, ok := h.deps.Gate.OwnerFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	h.render(w, h.termTmpl, map[string]string{"Owner": owner})
}

func (h *pagesHandler) render(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := t.Execute(w, data); err != nil {
		h.deps.Logger.Error("render page", "err", err)
	}
}
