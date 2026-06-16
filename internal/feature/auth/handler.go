// Package auth is a removable starter feature: email/password registration and
// login backed by server-side (DB) sessions, plus a RequireAuth wrapper other
// routes use to protect themselves. It demonstrates auth WITHOUT touching Core —
// it defines its own context key, exposes its own route wrapper, and reuses the
// global CSRF + security middleware.
//
// To remove it: delete this package, delete its migration, delete its sqlc block
// in sqlc.yaml, and delete the one line in internal/feature/registry.
//
// Password hashing is stdlib PBKDF2 (see password.go) to keep the template free
// of new runtime dependencies; swap to argon2id if you prefer.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/example/app/internal/core/app"
	"github.com/example/app/internal/core/assets"
	"github.com/example/app/internal/core/httpx"
	"github.com/example/app/internal/core/validate"
	"github.com/example/app/internal/feature/auth/store"
	"github.com/example/app/internal/view/layout"
)

const (
	minPasswordLen = 8
	maxEmailLen    = 254
	dbTimeout      = 5 * time.Second
)

// ctxKey is the feature's private context-key type. Core's httpx keys are
// unexported, which correctly forces a feature to define its own.
type ctxKey int

const userKey ctxKey = 0

// dummyHash equalizes verify timing on the unknown-email path so response timing
// does not reveal whether an account exists.
var dummyHash, _ = hashPassword("timing-equalization-placeholder")

// Module bundles the feature's dependencies and handlers. It depends on the
// generated store.Querier INTERFACE so tests inject a DB-free fake.
type Module struct {
	log    *slog.Logger
	assets *assets.Manager
	q      store.Querier
	prod   bool
}

// New constructs the feature from the stable Core dependencies.
func New(deps app.Deps) *Module {
	return &Module{
		log:    deps.Logger,
		assets: deps.Assets,
		q:      store.New(deps.Pool),
		prod:   deps.Config.IsProduction(),
	}
}

func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", m.loginForm)
	mux.HandleFunc("POST /login", m.login)
	mux.HandleFunc("POST /logout", m.logout)
	mux.HandleFunc("GET /register", m.registerForm)
	mux.HandleFunc("POST /register", m.register)
	mux.HandleFunc("GET /account", m.RequireAuth(m.account)) // demo protected route
}

func (m *Module) loginForm(w http.ResponseWriter, r *http.Request) {
	m.render(w, r, http.StatusOK, LoginPage(m.assets, "", ""))
}

func (m *Module) login(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	email := normalizeEmail(r.FormValue("email"))
	password := r.FormValue("password")

	user, err := m.q.GetUserByEmail(ctx, email)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		_ = verifyPassword(dummyHash, password) // equalize timing; no enumeration
		m.render(w, r, http.StatusUnauthorized, LoginPage(m.assets, email, "Invalid email or password."))
		return
	case err != nil:
		m.fail(w, r, err)
		return
	}

	if !verifyPassword(user.PasswordHash, password) {
		m.render(w, r, http.StatusUnauthorized, LoginPage(m.assets, email, "Invalid email or password."))
		return
	}

	if err := m.startSession(ctx, w, user.ID); err != nil {
		m.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (m *Module) logout(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	if c, err := r.Cookie(sessionCookieName(m.prod)); err == nil && c.Value != "" {
		if err := m.q.DeleteSession(ctx, hashToken(c.Value)); err != nil {
			m.log.ErrorContext(ctx, "delete session", slog.Any("error", err))
		}
	}
	clearSessionCookie(w, m.prod)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (m *Module) registerForm(w http.ResponseWriter, r *http.Request) {
	m.render(w, r, http.StatusOK, RegisterPage(m.assets, "", ""))
}

func (m *Module) register(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	email := normalizeEmail(r.FormValue("email"))
	password := r.FormValue("password")

	v := validate.New()
	v.Required("email", email)
	v.Email("email", email)
	v.MaxLen("email", email, maxEmailLen)
	v.MinLen("password", password, minPasswordLen)
	if !v.Valid() {
		m.render(w, r, http.StatusUnprocessableEntity, RegisterPage(m.assets, email, firstError(v)))
		return
	}

	hash, err := hashPassword(password)
	if err != nil {
		m.fail(w, r, err)
		return
	}

	user, err := m.q.CreateUser(ctx, store.CreateUserParams{Email: email, PasswordHash: hash})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			m.render(w, r, http.StatusUnprocessableEntity, RegisterPage(m.assets, email, "That email is already registered."))
			return
		}
		m.fail(w, r, err)
		return
	}

	if err := m.startSession(ctx, w, user.ID); err != nil {
		m.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (m *Module) account(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	data := layout.ShellData{Title: "Account · App", Path: r.URL.Path, UserName: user.Email, DisplayName: "App"}
	m.render(w, r, http.StatusOK, AccountPage(m.assets, user, data))
}

// RequireAuth wraps a handler so it runs only for an authenticated request: it
// loads the session user into the context (read via CurrentUser) or redirects to
// /login. Other features import and apply this to protect their own routes.
func (m *Module) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName(m.prod))
		if err != nil || c.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
		defer cancel()
		user, err := m.q.GetSessionUser(ctx, hashToken(c.Value))
		if err != nil {
			// Expired or unknown session: clear the stale cookie and send to login.
			clearSessionCookie(w, m.prod)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	}
}

// CurrentUser returns the authenticated user stored by RequireAuth, if any.
func CurrentUser(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(userKey).(store.User)
	return u, ok
}

// startSession mints a fresh session token (minting fresh on every login also
// prevents fixation) and sets the hardened cookie.
func (m *Module) startSession(ctx context.Context, w http.ResponseWriter, userID int64) error {
	token, hash := newSessionToken()
	expires := time.Now().Add(sessionTTL)
	if err := m.q.CreateSession(ctx, store.CreateSessionParams{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: expires,
	}); err != nil {
		return err
	}
	setSessionCookie(w, m.prod, token, expires)
	return nil
}

func (m *Module) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	if err := httpx.RenderHTML(w, r, status, c); err != nil {
		m.log.ErrorContext(r.Context(), "render auth view", slog.Any("error", err))
	}
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	code := httpx.StatusFor(err)
	if code >= 500 {
		m.log.ErrorContext(r.Context(), "auth handler error", slog.Any("error", err))
	}
	http.Error(w, http.StatusText(code), code)
}

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func firstError(v *validate.Validator) string {
	for _, field := range []string{"email", "password"} {
		if msg, ok := v.Errors[field]; ok {
			return strings.ToUpper(field[:1]) + field[1:] + " " + msg + "."
		}
	}
	return "Please check your input."
}
