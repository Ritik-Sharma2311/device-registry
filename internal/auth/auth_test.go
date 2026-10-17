package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newAuth(t *testing.T) *Authenticator {
	t.Helper()
	a, err := New("0123456789abcdef-secret", time.Minute, []User{
		{"ann", "pw-admin", RoleAdmin}, {"vic", "pw-view", RoleViewer},
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func call(h http.Handler, token string) int {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestLoginAndVerify(t *testing.T) {
	a := newAuth(t)
	tok, err := a.Login("ann", "pw-admin")
	if err != nil {
		t.Fatal(err)
	}
	c, err := a.Verify(tok)
	if err != nil || c.Role != RoleAdmin || c.Subject != "ann" {
		t.Fatalf("claims %+v err %v", c, err)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	a := newAuth(t)
	for _, c := range [][2]string{{"ann", "wrong"}, {"nobody", "pw-admin"}, {"ann", ""}, {"", ""}} {
		if _, err := a.Login(c[0], c[1]); err == nil {
			t.Fatalf("login %v should fail", c)
		}
	}
}

func TestRequireStatusCodes(t *testing.T) {
	a := newAuth(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	adminOnly := a.Require(ok, RoleAdmin)
	anyRole := a.Require(ok, RoleAdmin, RoleViewer)

	admin, _ := a.Login("ann", "pw-admin")
	viewer, _ := a.Login("vic", "pw-view")

	cases := []struct {
		name  string
		h     http.Handler
		token string
		want  int
	}{
		{"no token", adminOnly, "", 401},
		{"garbage token", adminOnly, "abc.def.ghi", 401},
		{"viewer on admin route", adminOnly, viewer, 403},
		{"admin on admin route", adminOnly, admin, 200},
		{"viewer on shared route", anyRole, viewer, 200},
	}
	for _, c := range cases {
		if got := call(c.h, c.token); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	a := newAuth(t)
	tok, _ := a.Login("ann", "pw-admin")
	a.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := a.Verify(tok); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

func TestRejectsAlgNoneAndWrongSecret(t *testing.T) {
	a := newAuth(t)
	claims := Claims{Role: RoleAdmin, RegisteredClaims: jwt.RegisteredClaims{
		Subject: "evil", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := a.Verify(none); err == nil {
		t.Fatal("alg none must be rejected")
	}
	forged, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("a-different-secret-xx"))
	if _, err := a.Verify(forged); err == nil {
		t.Fatal("token signed with another secret must be rejected")
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New("short", time.Minute, nil); err == nil {
		t.Fatal("short secret should fail")
	}
	if _, err := New("0123456789abcdef", time.Minute, []User{{"x", "y", "root"}}); err == nil {
		t.Fatal("unknown role should fail")
	}
}

func TestParseUsers(t *testing.T) {
	us, err := ParseUsers("ann:pw:admin, vic:pw2:viewer")
	if err != nil || len(us) != 2 || us[1].Role != RoleViewer {
		t.Fatalf("%+v %v", us, err)
	}
	if _, err := ParseUsers("bad"); err == nil {
		t.Fatal("bad spec should fail")
	}
}
