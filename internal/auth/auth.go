// Package auth issues and verifies short-lived JWTs and guards HTTP routes.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// User is a configured login.
type User struct {
	Name     string
	Password string
	Role     string
}

// Claims are the JWT payload.
type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type Authenticator struct {
	secret []byte
	ttl    time.Duration
	users  map[string]User
	now    func() time.Time
}

// New requires a secret of at least 16 bytes.
func New(secret string, ttl time.Duration, users []User) (*Authenticator, error) {
	if len(secret) < 16 {
		return nil, errors.New("auth: secret must be at least 16 characters")
	}
	m := make(map[string]User, len(users))
	for _, u := range users {
		if u.Role != RoleAdmin && u.Role != RoleViewer {
			return nil, fmt.Errorf("auth: user %q has unknown role %q", u.Name, u.Role)
		}
		m[u.Name] = u
	}
	return &Authenticator{secret: []byte(secret), ttl: ttl, users: m, now: time.Now}, nil
}

// ParseUsers reads "name:password:role,name:password:role".
func ParseUsers(spec string) ([]User, error) {
	var out []User
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		f := strings.Split(part, ":")
		if len(f) != 3 {
			return nil, fmt.Errorf("auth: bad user spec %q, want name:password:role", part)
		}
		out = append(out, User{Name: f[0], Password: f[1], Role: f[2]})
	}
	return out, nil
}

// Login checks credentials and returns a signed token.
func (a *Authenticator) Login(name, password string) (string, error) {
	u, ok := a.users[name]
	// Compare even for unknown users so response time does not reveal which names exist.
	want := u.Password
	if !ok {
		want = "\x00"
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(want)) != 1 || !ok {
		return "", errors.New("invalid credentials")
	}
	now := a.now()
	claims := Claims{
		Role: u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.Name,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.secret)
}

// Verify parses and validates a token. Only HS256 is accepted, which blocks
// the classic "alg: none" and algorithm-swap attacks.
func (a *Authenticator) Verify(token string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return a.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(a.now), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return &c, nil
}

type ctxKey struct{}

// ClaimsFrom returns the verified claims placed in ctx by Require.
func ClaimsFrom(ctx context.Context) *Claims {
	c, _ := ctx.Value(ctxKey{}).(*Claims)
	return c
}

// Require rejects requests without a valid Bearer token (401) or whose role
// is not in roles (403).
func (a *Authenticator) Require(next http.Handler, roles ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		tok, found := strings.CutPrefix(h, "Bearer ")
		if !found || tok == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
			return
		}
		claims, err := a.Verify(tok)
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}
		for _, role := range roles {
			if claims.Role == role {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, claims)))
				return
			}
		}
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	})
}
