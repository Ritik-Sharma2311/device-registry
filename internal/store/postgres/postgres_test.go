package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
	"github.com/Ritik-Sharma2311/device-registry/internal/store/storetest"
)

// Needs a Postgres, e.g.
// POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/postgres go test ./...
func TestContract(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set")
	}
	storetest.Run(t, func(t *testing.T) device.Store {
		s, err := Open(context.Background(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(context.Background(), `TRUNCATE devices`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		return s
	})
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set")
	}
	for i := 0; i < 2; i++ {
		s, err := Open(context.Background(), dsn)
		if err != nil {
			t.Fatalf("open #%d: %v", i+1, err)
		}
		s.Close()
	}
}
