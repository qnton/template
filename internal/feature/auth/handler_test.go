package auth

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/example/app/internal/core/assets"
	"github.com/example/app/internal/feature/auth/store"
)

// testPasswordHash is computed once (PBKDF2 is intentionally slow) and reused.
var testPasswordHash = mustHash("correct horse battery staple")

func mustHash(p string) string {
	h, err := hashPassword(p)
	if err != nil {
		panic(err)
	}
	return h
}

// fakeStore is an in-memory store.Querier for fast, DB-free handler tests.
type fakeStore struct {
	usersByEmail map[string]store.User
	usersByID    map[int64]store.User
	sessions     map[string]int64 // string(token_hash) -> user id
	nextID       int64
}

var _ store.Querier = (*fakeStore)(nil)

func newFakeStore() *fakeStore {
	return &fakeStore{
		usersByEmail: map[string]store.User{},
		usersByID:    map[int64]store.User{},
		sessions:     map[string]int64{},
		nextID:       1,
	}
}

func (f *fakeStore) seedUser(id int64, email, hash string) store.User {
	u := store.User{ID: id, Email: email, PasswordHash: hash, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.usersByEmail[email] = u
	f.usersByID[id] = u
	if id >= f.nextID {
		f.nextID = id + 1
	}
	return u
}

func (f *fakeStore) CreateUser(_ context.Context, arg store.CreateUserParams) (store.User, error) {
	if _, exists := f.usersByEmail[arg.Email]; exists {
		return store.User{}, &pgconn.PgError{Code: "23505"} // unique_violation
	}
	u := store.User{
		ID:           f.nextID,
		Email:        arg.Email,
		PasswordHash: arg.PasswordHash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	f.nextID++
	f.usersByEmail[u.Email] = u
	f.usersByID[u.ID] = u
	return u, nil
}

func (f *fakeStore) GetUserByEmail(_ context.Context, email string) (store.User, error) {
	if u, ok := f.usersByEmail[email]; ok {
		return u, nil
	}
	return store.User{}, pgx.ErrNoRows
}

func (f *fakeStore) CreateSession(_ context.Context, arg store.CreateSessionParams) error {
	f.sessions[string(arg.TokenHash)] = arg.UserID
	return nil
}

func (f *fakeStore) GetSessionUser(_ context.Context, tokenHash []byte) (store.User, error) {
	if id, ok := f.sessions[string(tokenHash)]; ok {
		if u, ok := f.usersByID[id]; ok {
			return u, nil
		}
	}
	return store.User{}, pgx.ErrNoRows
}

func (f *fakeStore) DeleteSession(_ context.Context, tokenHash []byte) error {
	delete(f.sessions, string(tokenHash))
	return nil
}

func (f *fakeStore) DeleteExpiredSessions(_ context.Context) error { return nil }

func testAssets(tb testing.TB) *assets.Manager {
	tb.Helper()
	m, err := assets.NewManager(fstest.MapFS{
		"css/app.css":                 {Data: []byte("/*css*/")},
		"js/htmx.min.js":              {Data: []byte("/*htmx*/")},
		"js/core.mjs":                 {Data: []byte("/*core*/")},
		"js/islands/theme-toggle.mjs": {Data: []byte("/*toggle*/")},
	})
	if err != nil {
		tb.Fatalf("assets: %v", err)
	}
	return m
}

func newTestModule(tb testing.TB, f *fakeStore) (*Module, *http.ServeMux) {
	tb.Helper()
	m := &Module{
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		assets: testAssets(tb),
		q:      f,
		prod:   false,
	}
	mux := http.NewServeMux()
	m.Routes(mux)
	return m, mux
}

func postForm(mux *http.ServeMux, path string, values url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func findCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestLoginSuccess(t *testing.T) {
	f := newFakeStore()
	f.seedUser(1, "user@example.com", testPasswordHash)
	_, mux := newTestModule(t, f)

	rec := postForm(mux, "/login", url.Values{
		"email":    {"user@example.com"},
		"password": {"correct horse battery staple"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/account" {
		t.Errorf("Location = %q, want /account", loc)
	}
	c := findCookie(rec, "session")
	if c == nil || c.Value == "" {
		t.Fatal("session cookie not set")
	}
	if !c.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if len(f.sessions) != 1 {
		t.Errorf("sessions = %d, want 1", len(f.sessions))
	}
}

func TestLoginWrongPassword(t *testing.T) {
	f := newFakeStore()
	f.seedUser(1, "user@example.com", testPasswordHash)
	_, mux := newTestModule(t, f)

	rec := postForm(mux, "/login", url.Values{
		"email":    {"user@example.com"},
		"password": {"not the password"},
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if findCookie(rec, "session") != nil {
		t.Error("no session cookie expected on failed login")
	}
	if len(f.sessions) != 0 {
		t.Error("no session expected on failed login")
	}
	if !strings.Contains(rec.Body.String(), "Invalid email or password") {
		t.Error("expected the generic error message")
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	f := newFakeStore()
	_, mux := newTestModule(t, f)

	rec := postForm(mux, "/login", url.Values{
		"email":    {"nobody@example.com"},
		"password": {"whatever-pass"},
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid email or password") {
		t.Error("expected the generic error message (no user enumeration)")
	}
}

func TestLogoutClearsSession(t *testing.T) {
	f := newFakeStore()
	f.seedUser(1, "user@example.com", testPasswordHash)
	token, hash := newSessionToken()
	f.sessions[string(hash)] = 1
	_, mux := newTestModule(t, f)

	rec := postForm(mux, "/logout", url.Values{}, &http.Cookie{Name: "session", Value: token})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
	if len(f.sessions) != 0 {
		t.Error("session not deleted on logout")
	}
	if c := findCookie(rec, "session"); c == nil || c.MaxAge >= 0 {
		t.Errorf("expected a cleared session cookie (MaxAge<0); got %+v", c)
	}
}

func TestRequireAuthRedirectsAnonymous(t *testing.T) {
	f := newFakeStore()
	_, mux := newTestModule(t, f)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/account", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestRequireAuthAllowsValidSession(t *testing.T) {
	f := newFakeStore()
	f.seedUser(1, "user@example.com", testPasswordHash)
	token, hash := newSessionToken()
	f.sessions[string(hash)] = 1
	_, mux := newTestModule(t, f)

	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "user@example.com") {
		t.Error("account page should show the user's email")
	}
}

func TestRegisterCreatesUser(t *testing.T) {
	f := newFakeStore()
	_, mux := newTestModule(t, f)

	rec := postForm(mux, "/register", url.Values{
		"email":    {"New@Example.com"}, // mixed case → normalized
		"password": {"longenoughpass"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	u, ok := f.usersByEmail["new@example.com"]
	if !ok {
		t.Fatal("user not created (email should be normalized to lowercase)")
	}
	if !verifyPassword(u.PasswordHash, "longenoughpass") {
		t.Error("stored password hash does not verify")
	}
	if len(f.sessions) != 1 {
		t.Error("expected an auto-login session after register")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	f := newFakeStore()
	f.seedUser(1, "taken@example.com", testPasswordHash)
	_, mux := newTestModule(t, f)

	rec := postForm(mux, "/register", url.Values{
		"email":    {"taken@example.com"},
		"password": {"longenoughpass"},
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already registered") {
		t.Error("expected the duplicate-email message")
	}
}

func TestRegisterRejectsShortPassword(t *testing.T) {
	f := newFakeStore()
	_, mux := newTestModule(t, f)

	rec := postForm(mux, "/register", url.Values{
		"email":    {"a@example.com"},
		"password": {"short"},
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if len(f.usersByEmail) != 0 {
		t.Error("user must not be created with invalid input")
	}
}
