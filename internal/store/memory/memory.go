// Package memory is an in-process Store for tests and local runs.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
)

type Store struct {
	mu      sync.RWMutex
	devices map[string]device.Device
}

func New() *Store { return &Store{devices: make(map[string]device.Device)} }

func (s *Store) nameTaken(name, exceptID string) bool {
	for id, d := range s.devices {
		if d.Name == name && id != exceptID {
			return true
		}
	}
	return false
}

func (s *Store) Create(_ context.Context, d device.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nameTaken(d.Name, "") {
		return device.ErrConflict
	}
	s.devices[d.ID] = d
	return nil
}

func (s *Store) Get(_ context.Context, id string) (device.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[id]
	if !ok {
		return device.Device{}, device.ErrNotFound
	}
	return d, nil
}

func (s *Store) List(_ context.Context, f device.Filter) ([]device.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []device.Device
	for _, d := range s.devices {
		if d.ID <= f.AfterID || (f.Kind != "" && d.Kind != f.Kind) || (f.Status != "" && d.Status != f.Status) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *Store) Update(_ context.Context, in device.Device) (device.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.devices[in.ID]
	if !ok {
		return device.Device{}, device.ErrNotFound
	}
	if cur.Version != in.Version {
		return device.Device{}, device.ErrVersionConflict
	}
	if s.nameTaken(in.Name, in.ID) {
		return device.Device{}, device.ErrConflict
	}
	cur.Name, cur.Kind, cur.Address, cur.Status = in.Name, in.Kind, in.Address, in.Status
	cur.UpdatedAt = in.UpdatedAt
	cur.Version++
	s.devices[cur.ID] = cur
	return cur, nil
}

func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; !ok {
		return device.ErrNotFound
	}
	delete(s.devices, id)
	return nil
}

func (s *Store) Close() {}
