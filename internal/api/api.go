// Package api is the HTTP layer: routing, JSON, status codes and auth wiring.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Ritik-Sharma2311/device-registry/internal/auth"
	"github.com/Ritik-Sharma2311/device-registry/internal/device"
)

const maxBody = 1 << 20

type API struct {
	svc  *device.Service
	auth *auth.Authenticator
}

func New(svc *device.Service, a *auth.Authenticator) *API {
	return &API{svc: svc, auth: a}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	both := []string{auth.RoleAdmin, auth.RoleViewer}
	admin := []string{auth.RoleAdmin}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /auth/token", a.login)
	mux.Handle("GET /devices", a.auth.Require(http.HandlerFunc(a.list), both...))
	mux.Handle("GET /devices/{id}", a.auth.Require(http.HandlerFunc(a.get), both...))
	mux.Handle("POST /devices", a.auth.Require(http.HandlerFunc(a.create), admin...))
	mux.Handle("PUT /devices/{id}", a.auth.Require(http.HandlerFunc(a.update), admin...))
	mux.Handle("DELETE /devices/{id}", a.auth.Require(http.HandlerFunc(a.remove), admin...))
	return mux
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	tok, err := a.auth.Login(body.Username, body.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var in device.Input
	if !decode(w, r, &in) {
		return
	}
	d, err := a.svc.Create(r.Context(), in)
	if err != nil {
		a.fail(w, err)
		return
	}
	w.Header().Set("Location", "/devices/"+d.ID)
	w.Header().Set("ETag", etag(d.Version))
	writeJSON(w, http.StatusCreated, d)
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	d, err := a.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag(d.Version))
	writeJSON(w, http.StatusOK, d)
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	items, next, err := a.svc.List(r.Context(), device.Filter{
		Kind: q.Get("kind"), Status: q.Get("status"), AfterID: q.Get("after"), Limit: limit,
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	if items == nil {
		items = []device.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_after": next})
}

// update needs If-Match with the version the client last read. This stops one
// client silently overwriting another's change (lost update).
func (a *API) update(w http.ResponseWriter, r *http.Request) {
	raw := r.Header.Get("If-Match")
	if raw == "" {
		writeErr(w, http.StatusPreconditionRequired, "If-Match header with the current version is required")
		return
	}
	version, err := strconv.ParseInt(strings.Trim(raw, `"`), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "If-Match must be the version number, e.g. \"3\"")
		return
	}
	var in device.Input
	if !decode(w, r, &in) {
		return
	}
	d, err := a.svc.Update(r.Context(), r.PathValue("id"), version, in)
	if err != nil {
		a.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag(d.Version))
	writeJSON(w, http.StatusOK, d)
}

func (a *API) remove(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fail maps domain errors to HTTP statuses. Unknown errors are logged by the
// caller's infrastructure and returned as a generic 500 so internals do not leak.
func (a *API) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, device.ErrInvalid):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, device.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, device.ErrConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, device.ErrVersionConflict):
		writeErr(w, http.StatusPreconditionFailed, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func etag(v int64) string { return `"` + strconv.FormatInt(v, 10) + `"` }

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
