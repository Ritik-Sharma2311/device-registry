// Package events publishes device changes to NATS.
package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
)

// SubjectPrefix is prepended to the event type: devices.created, devices.updated, devices.deleted.
const SubjectPrefix = "devices."

type NATS struct{ conn *nats.Conn }

// Connect opens a connection that keeps retrying if the server goes away.
func Connect(url string) (*NATS, error) {
	nc, err := nats.Connect(url, nats.Name("device-registry"), nats.MaxReconnects(-1))
	if err != nil {
		return nil, err
	}
	return &NATS{conn: nc}, nil
}

func (n *NATS) Publish(_ context.Context, e device.Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := n.conn.Publish(SubjectPrefix+e.Type, body); err != nil {
		return fmt.Errorf("nats publish: %w", err)
	}
	return nil
}

func (n *NATS) Close() {
	_ = n.conn.Drain()
}
