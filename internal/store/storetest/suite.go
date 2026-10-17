// Package storetest is a contract test suite that every device.Store
// implementation must pass, so backends stay interchangeable.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
)

func mk(id, name string) device.Device {
	now := device.Now()
	return device.Device{ID: id, Name: name, Kind: "camera", Address: "10.0.0.1:554", Status: "online", Version: 1, CreatedAt: now, UpdatedAt: now}
}

// Run executes the suite. newStore must return a fresh, empty store.
func Run(t *testing.T, newStore func(t *testing.T) device.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("create and get round trip", func(t *testing.T) {
		s := newStore(t)
		d := mk("a1", "front-door")
		if err := s.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(ctx, "a1")
		if err != nil {
			t.Fatal(err)
		}
		if got != d {
			t.Fatalf("round trip changed the device:\n got %+v\nwant %+v", got, d)
		}
	})

	t.Run("get missing is ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.Get(ctx, "nope"); !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("duplicate name is ErrConflict", func(t *testing.T) {
		s := newStore(t)
		_ = s.Create(ctx, mk("a1", "same"))
		if err := s.Create(ctx, mk("a2", "same")); !errors.Is(err, device.ErrConflict) {
			t.Fatalf("want ErrConflict, got %v", err)
		}
	})

	t.Run("update bumps version and stores fields", func(t *testing.T) {
		s := newStore(t)
		_ = s.Create(ctx, mk("a1", "cam"))
		upd := mk("a1", "cam-renamed")
		upd.Status = "offline"
		upd.Version = 1
		got, err := s.Update(ctx, upd)
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != 2 || got.Name != "cam-renamed" || got.Status != "offline" {
			t.Fatalf("unexpected result %+v", got)
		}
		stored, _ := s.Get(ctx, "a1")
		if stored != got {
			t.Fatalf("returned and stored differ:\n%+v\n%+v", got, stored)
		}
	})

	t.Run("update with stale version is ErrVersionConflict", func(t *testing.T) {
		s := newStore(t)
		_ = s.Create(ctx, mk("a1", "cam"))
		upd := mk("a1", "cam")
		upd.Version = 1
		if _, err := s.Update(ctx, upd); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Update(ctx, upd); !errors.Is(err, device.ErrVersionConflict) {
			t.Fatalf("want ErrVersionConflict, got %v", err)
		}
	})

	t.Run("update missing is ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.Update(ctx, mk("ghost", "g")); !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("update to a taken name is ErrConflict", func(t *testing.T) {
		s := newStore(t)
		_ = s.Create(ctx, mk("a1", "one"))
		_ = s.Create(ctx, mk("a2", "two"))
		upd := mk("a2", "one")
		if _, err := s.Update(ctx, upd); !errors.Is(err, device.ErrConflict) {
			t.Fatalf("want ErrConflict, got %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		s := newStore(t)
		_ = s.Create(ctx, mk("a1", "cam"))
		if err := s.Delete(ctx, "a1"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(ctx, "a1"); !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("want ErrNotFound after delete, got %v", err)
		}
		if err := s.Delete(ctx, "a1"); !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("second delete: want ErrNotFound, got %v", err)
		}
	})

	t.Run("list filters, orders and pages", func(t *testing.T) {
		s := newStore(t)
		for i := 1; i <= 6; i++ {
			d := mk(fmt.Sprintf("id%d", i), fmt.Sprintf("dev%d", i))
			if i%2 == 0 {
				d.Kind = "sensor"
			}
			if i > 4 {
				d.Status = "offline"
			}
			if err := s.Create(ctx, d); err != nil {
				t.Fatal(err)
			}
		}
		all, _ := s.List(ctx, device.Filter{})
		if len(all) != 6 || all[0].ID != "id1" || all[5].ID != "id6" {
			t.Fatalf("want 6 ordered devices, got %+v", ids(all))
		}
		if got, _ := s.List(ctx, device.Filter{Kind: "sensor"}); len(got) != 3 {
			t.Fatalf("kind filter: want 3, got %v", ids(got))
		}
		if got, _ := s.List(ctx, device.Filter{Status: "offline"}); len(got) != 2 {
			t.Fatalf("status filter: want 2, got %v", ids(got))
		}
		page, _ := s.List(ctx, device.Filter{AfterID: "id2", Limit: 2})
		if len(page) != 2 || page[0].ID != "id3" || page[1].ID != "id4" {
			t.Fatalf("paging: want id3,id4 got %v", ids(page))
		}
	})

	t.Run("concurrent updates: exactly one wins per version", func(t *testing.T) {
		s := newStore(t)
		_ = s.Create(ctx, mk("a1", "cam"))
		var ok, conflict atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				upd := mk("a1", fmt.Sprintf("cam-%d", i))
				upd.Version = 1
				_, err := s.Update(ctx, upd)
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, device.ErrVersionConflict):
					conflict.Add(1)
				default:
					t.Error(err)
				}
			}(i)
		}
		wg.Wait()
		if ok.Load() != 1 || conflict.Load() != 19 {
			t.Fatalf("ok=%d conflict=%d, want 1/19", ok.Load(), conflict.Load())
		}
		if got, _ := s.Get(ctx, "a1"); got.Version != 2 {
			t.Fatalf("version %d, want 2", got.Version)
		}
	})
}

func ids(ds []device.Device) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.ID
	}
	return out
}
