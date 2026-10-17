package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ritik-Sharma2311/device-registry/internal/auth"
	"github.com/Ritik-Sharma2311/device-registry/internal/device"
	"github.com/Ritik-Sharma2311/device-registry/internal/store/memory"
)

type recorder struct {
	mu     sync.Mutex
	events []device.Event
}

func (r *recorder) Publish(_ context.Context, e device.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

type env struct {
	srv    *httptest.Server
	admin  string
	viewer string
	events *recorder
}

func setup(t *testing.T) *env {
	t.Helper()
	a, err := auth.New("0123456789abcdef-secret", time.Minute, []auth.User{
		{Name: "ann", Password: "pw-admin", Role: auth.RoleAdmin},
		{Name: "vic", Password: "pw-view", Role: auth.RoleViewer},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	srv := httptest.NewServer(New(device.NewService(memory.New(), rec), a).Handler())
	t.Cleanup(srv.Close)
	e := &env{srv: srv, events: rec}
	e.admin, _ = a.Login("ann", "pw-admin")
	e.viewer, _ = a.Login("vic", "pw-view")
	return e
}

func (e *env) do(t *testing.T, method, path, token string, body any, hdr map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

var cam = map[string]string{"name": "front-door", "kind": "camera", "address": "10.0.0.5:554"}

func TestAuthRequired(t *testing.T) {
	e := setup(t)
	if r, _ := e.do(t, "GET", "/devices", "", nil, nil); r.StatusCode != 401 {
		t.Fatalf("no token: %d", r.StatusCode)
	}
	if r, _ := e.do(t, "POST", "/devices", e.viewer, cam, nil); r.StatusCode != 403 {
		t.Fatalf("viewer create: %d", r.StatusCode)
	}
	if r, _ := e.do(t, "GET", "/devices", e.viewer, nil, nil); r.StatusCode != 200 {
		t.Fatalf("viewer list: %d", r.StatusCode)
	}
}

func TestLoginEndpoint(t *testing.T) {
	e := setup(t)
	r, out := e.do(t, "POST", "/auth/token", "", map[string]string{"username": "ann", "password": "pw-admin"}, nil)
	if r.StatusCode != 200 || out["token"] == "" {
		t.Fatalf("login: %d %v", r.StatusCode, out)
	}
	if r, _ := e.do(t, "POST", "/auth/token", "", map[string]string{"username": "ann", "password": "bad"}, nil); r.StatusCode != 401 {
		t.Fatalf("bad login: %d", r.StatusCode)
	}
}

func TestCreateGetUpdateDeleteFlow(t *testing.T) {
	e := setup(t)

	r, d := e.do(t, "POST", "/devices", e.admin, cam, nil)
	if r.StatusCode != 201 || r.Header.Get("ETag") != `"1"` || d["status"] != "unknown" {
		t.Fatalf("create: %d %v etag=%s", r.StatusCode, d, r.Header.Get("ETag"))
	}
	id := d["id"].(string)
	if r.Header.Get("Location") != "/devices/"+id {
		t.Fatalf("location %q", r.Header.Get("Location"))
	}

	if r, _ := e.do(t, "GET", "/devices/"+id, e.viewer, nil, nil); r.StatusCode != 200 {
		t.Fatalf("get: %d", r.StatusCode)
	}

	upd := map[string]string{"name": "front-door", "kind": "camera", "address": "10.0.0.5:554", "status": "online"}
	r, d = e.do(t, "PUT", "/devices/"+id, e.admin, upd, map[string]string{"If-Match": `"1"`})
	if r.StatusCode != 200 || d["version"].(float64) != 2 || r.Header.Get("ETag") != `"2"` {
		t.Fatalf("update: %d %v", r.StatusCode, d)
	}

	if r, _ := e.do(t, "DELETE", "/devices/"+id, e.admin, nil, nil); r.StatusCode != 204 {
		t.Fatalf("delete: %d", r.StatusCode)
	}
	if r, _ := e.do(t, "GET", "/devices/"+id, e.admin, nil, nil); r.StatusCode != 404 {
		t.Fatalf("get after delete: %d", r.StatusCode)
	}

	var types []string
	for _, ev := range e.events.events {
		types = append(types, ev.Type)
	}
	if strings.Join(types, ",") != "created,updated,deleted" {
		t.Fatalf("events %v", types)
	}
}

func TestUpdateConcurrencyHeaders(t *testing.T) {
	e := setup(t)
	_, d := e.do(t, "POST", "/devices", e.admin, cam, nil)
	id := d["id"].(string)

	if r, _ := e.do(t, "PUT", "/devices/"+id, e.admin, cam, nil); r.StatusCode != 428 {
		t.Fatalf("missing If-Match: %d, want 428", r.StatusCode)
	}
	if r, _ := e.do(t, "PUT", "/devices/"+id, e.admin, cam, map[string]string{"If-Match": "abc"}); r.StatusCode != 400 {
		t.Fatalf("bad If-Match: %d, want 400", r.StatusCode)
	}
	if r, _ := e.do(t, "PUT", "/devices/"+id, e.admin, cam, map[string]string{"If-Match": `"9"`}); r.StatusCode != 412 {
		t.Fatalf("stale If-Match: %d, want 412", r.StatusCode)
	}
}

func TestErrorStatuses(t *testing.T) {
	e := setup(t)
	if r, _ := e.do(t, "POST", "/devices", e.admin, map[string]string{"name": "", "kind": "camera", "address": "x"}, nil); r.StatusCode != 422 {
		t.Fatalf("invalid: %d", r.StatusCode)
	}
	if r, _ := e.do(t, "POST", "/devices", e.admin, map[string]string{"name": "n", "kind": "toaster", "address": "x"}, nil); r.StatusCode != 422 {
		t.Fatalf("bad kind: %d", r.StatusCode)
	}
	e.do(t, "POST", "/devices", e.admin, cam, nil)
	if r, _ := e.do(t, "POST", "/devices", e.admin, cam, nil); r.StatusCode != 409 {
		t.Fatalf("duplicate: %d", r.StatusCode)
	}
	if r, _ := e.do(t, "POST", "/devices", e.admin, map[string]any{"name": "a", "kind": "camera", "address": "x", "extra": 1}, nil); r.StatusCode != 400 {
		t.Fatalf("unknown field: %d", r.StatusCode)
	}
	if r, _ := e.do(t, "GET", "/devices/missing", e.admin, nil, nil); r.StatusCode != 404 {
		t.Fatalf("missing: %d", r.StatusCode)
	}
}

func TestListPagination(t *testing.T) {
	e := setup(t)
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		e.do(t, "POST", "/devices", e.admin, map[string]string{"name": n, "kind": "sensor", "address": "x"}, nil)
	}
	seen := map[string]bool{}
	after := ""
	for pages := 0; pages < 10; pages++ {
		_, out := e.do(t, "GET", "/devices?limit=2&after="+after, e.viewer, nil, nil)
		items := out["items"].([]any)
		for _, it := range items {
			seen[it.(map[string]any)["id"].(string)] = true
		}
		next, _ := out["next_after"].(string)
		if next == "" {
			break
		}
		after = next
	}
	if len(seen) != 5 {
		t.Fatalf("paging visited %d devices, want 5", len(seen))
	}
}
