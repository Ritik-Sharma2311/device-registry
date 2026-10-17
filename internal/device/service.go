package device

import (
	"context"
	"log"

	"github.com/google/uuid"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

// Service applies validation and publishes events around a Store.
type Service struct {
	store Store
	pub   Publisher
}

// NewService returns a Service. pub may be nil to disable events.
func NewService(store Store, pub Publisher) *Service {
	return &Service{store: store, pub: pub}
}

func (s *Service) Create(ctx context.Context, in Input) (Device, error) {
	if err := in.Validate(); err != nil {
		return Device{}, err
	}
	now := Now()
	d := Device{
		ID: uuid.NewString(), Name: in.Name, Kind: in.Kind, Address: in.Address,
		Status: in.Status, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.Create(ctx, d); err != nil {
		return Device{}, err
	}
	s.emit(ctx, "created", d)
	return d, nil
}

func (s *Service) Get(ctx context.Context, id string) (Device, error) {
	return s.store.Get(ctx, id)
}

// List returns one page and the ID to pass as AfterID for the next page
// ("" when there are no more).
func (s *Service) List(ctx context.Context, f Filter) ([]Device, string, error) {
	if f.Limit < 1 {
		f.Limit = defaultLimit
	}
	if f.Limit > maxLimit {
		f.Limit = maxLimit
	}
	// Ask for one extra row: if it comes back there is another page.
	f.Limit++
	items, err := s.store.List(ctx, f)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) == f.Limit {
		items = items[:f.Limit-1]
		next = items[len(items)-1].ID
	}
	return items, next, nil
}

// Update changes a device if version still matches what the caller last saw.
func (s *Service) Update(ctx context.Context, id string, version int64, in Input) (Device, error) {
	if err := in.Validate(); err != nil {
		return Device{}, err
	}
	d, err := s.store.Update(ctx, Device{
		ID: id, Name: in.Name, Kind: in.Kind, Address: in.Address,
		Status: in.Status, Version: version, UpdatedAt: Now(),
	})
	if err != nil {
		return Device{}, err
	}
	s.emit(ctx, "updated", d)
	return d, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	d, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	s.emit(ctx, "deleted", d)
	return nil
}

// emit is best effort: the change is already stored, so a broker outage is
// logged instead of failing the request.
func (s *Service) emit(ctx context.Context, typ string, d Device) {
	if s.pub == nil {
		return
	}
	if err := s.pub.Publish(ctx, Event{Type: typ, Device: d, At: Now()}); err != nil {
		log.Printf("publish %s for device %s failed: %v", typ, d.ID, err)
	}
}
