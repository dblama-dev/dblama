package driver

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

type registered struct {
	meta    DriverMeta
	connect ConnectFunc
}

// Registry maps driver IDs to their connect functions.
type Registry struct {
	drivers map[string]registered
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{drivers: make(map[string]registered)}
}

// Register adds a driver under its meta ID, replacing any existing
// registration with the same ID.
func (r *Registry) Register(m DriverMeta, connect ConnectFunc) {
	r.drivers[m.ID] = registered{meta: m, connect: connect}
}

// List returns the metas of all registered drivers, sorted by ID.
func (r *Registry) List() []DriverMeta {
	metas := make([]DriverMeta, 0, len(r.drivers))
	for _, reg := range r.drivers {
		metas = append(metas, reg.meta)
	}
	slices.SortFunc(metas, func(a, b DriverMeta) int {
		return strings.Compare(a.ID, b.ID)
	})
	return metas
}

// Connect establishes a connection via the driver registered under id.
func (r *Registry) Connect(ctx context.Context, id string, cfg ConnConfig) (Driver, error) {
	reg, ok := r.drivers[id]
	if !ok {
		return nil, fmt.Errorf("unknown driver %q", id)
	}
	return reg.connect(ctx, cfg)
}
