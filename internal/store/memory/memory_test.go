package memory

import (
	"testing"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
	"github.com/Ritik-Sharma2311/device-registry/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(*testing.T) device.Store { return New() })
}
