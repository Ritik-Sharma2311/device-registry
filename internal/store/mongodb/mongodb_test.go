package mongodb

import (
	"context"
	"os"
	"testing"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
	"github.com/Ritik-Sharma2311/device-registry/internal/store/storetest"
)

// Needs a MongoDB, e.g. MONGO_URI=mongodb://localhost:27017 go test ./...
func TestContract(t *testing.T) {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		t.Skip("MONGO_URI not set")
	}
	storetest.Run(t, func(t *testing.T) device.Store {
		s, err := Open(context.Background(), uri, "registry_test")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.coll.Drop(context.Background()); err != nil {
			t.Fatal(err)
		}
		// Dropping removes the indexes too, so recreate them.
		s.Close()
		s, err = Open(context.Background(), uri, "registry_test")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		return s
	})
}
