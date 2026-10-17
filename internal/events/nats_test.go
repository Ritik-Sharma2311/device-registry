package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
)

func startServer(t *testing.T) string {
	t.Helper()
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("embedded nats not ready")
	}
	t.Cleanup(s.Shutdown)
	return s.ClientURL()
}

func TestPublishDeliversJSONOnTypedSubject(t *testing.T) {
	url := startServer(t)

	sub, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ch := make(chan *nats.Msg, 1)
	if _, err := sub.ChanSubscribe("devices.*", ch); err != nil {
		t.Fatal(err)
	}
	sub.Flush()

	pub, err := Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	want := device.Event{Type: "updated", Device: device.Device{ID: "x1", Name: "cam", Version: 3}, At: device.Now()}
	if err := pub.Publish(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-ch:
		if msg.Subject != "devices.updated" {
			t.Fatalf("subject %q", msg.Subject)
		}
		var got device.Event
		if err := json.Unmarshal(msg.Data, &got); err != nil {
			t.Fatal(err)
		}
		if got.Device.ID != "x1" || got.Device.Version != 3 || got.Type != "updated" {
			t.Fatalf("unexpected event %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message received")
	}
}
