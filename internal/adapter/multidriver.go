package adapter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// metaDriverIndex tags candidates with the sub-driver that produced them.
const metaDriverIndex = "multidriver.index"

// MultiDriver composes several transport drivers (e.g. serial and TCP)
// under one Kind. Manager holds one driver per Kind, so combined discovery
// for a kind must go through a composite like this.
type MultiDriver struct {
	kind    Kind
	drivers []Driver
}

// NewMultiDriver returns a driver for kind that fans out to drivers.
func NewMultiDriver(kind Kind, drivers ...Driver) *MultiDriver {
	return &MultiDriver{kind: kind, drivers: append([]Driver(nil), drivers...)}
}

func (m *MultiDriver) Kind() Kind { return m.kind }

// Discover merges candidates from every sub-driver, tagging each with its
// origin so Open can route it. Errors are joined; partial results are kept.
func (m *MultiDriver) Discover(ctx context.Context) ([]Candidate, error) {
	var out []Candidate
	var errs []error
	for i, d := range m.drivers {
		cands, err := d.Discover(ctx)
		if err != nil {
			errs = append(errs, err)
		}
		for _, c := range cands {
			meta := make(map[string]string, len(c.Metadata)+1)
			for k, v := range c.Metadata {
				meta[k] = v
			}
			meta[metaDriverIndex] = strconv.Itoa(i)
			c.Metadata = meta
			c.Info.Kind = m.kind
			out = append(out, c)
		}
	}
	return out, errors.Join(errs...)
}

// Open routes c to the sub-driver that discovered it.
func (m *MultiDriver) Open(ctx context.Context, c Candidate) (Session, error) {
	i, err := strconv.Atoi(c.Metadata[metaDriverIndex])
	if err != nil || i < 0 || i >= len(m.drivers) {
		return nil, fmt.Errorf("%w: candidate %s not produced by this driver", ErrUnsupported, c.Info.ID)
	}
	return m.drivers[i].Open(ctx, c)
}
