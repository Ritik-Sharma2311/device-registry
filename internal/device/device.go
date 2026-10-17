// Package device holds the domain model, the storage contract and the
// business rules of the registry.
package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound        = errors.New("device not found")
	ErrConflict        = errors.New("a device with this name already exists")
	ErrVersionConflict = errors.New("device was modified by someone else")
	ErrInvalid         = errors.New("invalid device")
)

// Allowed values for Kind and Status.
var (
	Kinds    = []string{"camera", "nvr", "sensor", "gateway"}
	Statuses = []string{"online", "offline", "unknown"}
)

// Device is one managed piece of hardware.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Address   string    `json:"address"`
	Status    string    `json:"status"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Input is the writable part of a Device.
type Input struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Address string `json:"address"`
	Status  string `json:"status"`
}

// Filter narrows and pages a List call. Results are ordered by ID.
type Filter struct {
	Kind    string
	Status  string
	AfterID string // return only devices with ID greater than this
	Limit   int
}

// Store is the persistence contract. Every backend (memory, Postgres, MongoDB)
// implements it and is checked by the same contract tests.
type Store interface {
	// Create inserts d. It returns ErrConflict if the name is taken.
	Create(ctx context.Context, d Device) error
	Get(ctx context.Context, id string) (Device, error)
	List(ctx context.Context, f Filter) ([]Device, error)
	// Update replaces the writable fields of d.ID, but only if the stored
	// version equals d.Version. On success it returns the stored device with
	// the version incremented. Errors: ErrNotFound, ErrVersionConflict, ErrConflict.
	Update(ctx context.Context, d Device) (Device, error)
	Delete(ctx context.Context, id string) error
	Close()
}

// Event describes a change, published after it is stored.
type Event struct {
	Type   string    `json:"type"` // created, updated, deleted
	Device Device    `json:"device"`
	At     time.Time `json:"at"`
}

// Publisher sends events to other services.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

// Validate normalises in and checks it. Status defaults to "unknown".
func (in *Input) Validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	if in.Status == "" {
		in.Status = "unknown"
	}
	switch {
	case in.Name == "" || len(in.Name) > 100:
		return fmt.Errorf("%w: name must be 1-100 characters", ErrInvalid)
	case in.Address == "" || len(in.Address) > 255:
		return fmt.Errorf("%w: address must be 1-255 characters", ErrInvalid)
	case !contains(Kinds, in.Kind):
		return fmt.Errorf("%w: kind must be one of %s", ErrInvalid, strings.Join(Kinds, ", "))
	case !contains(Statuses, in.Status):
		return fmt.Errorf("%w: status must be one of %s", ErrInvalid, strings.Join(Statuses, ", "))
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// Now returns the current UTC time at millisecond precision, the finest
// resolution every backend can store, so values round-trip unchanged.
func Now() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }
