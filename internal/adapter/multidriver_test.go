package adapter

import (
	"context"
	"errors"
	"testing"
)

type fakeDriver struct {
	kind  Kind
	cands []Candidate
	err   error
	name  string
}

func (f *fakeDriver) Kind() Kind { return f.kind }

func (f *fakeDriver) Discover(context.Context) ([]Candidate, error) {
	out := make([]Candidate, len(f.cands))
	copy(out, f.cands)
	return out, f.err
}

func (f *fakeDriver) Open(context.Context, Candidate) (Session, error) {
	return nil, errors.New("opened by " + f.name)
}

func TestManagerKeepsPartialCompositeResults(t *testing.T) {
	failing := &fakeDriver{kind: KindELM327, err: errors.New("serial root unreadable"), name: "serial"}
	healthy := &fakeDriver{kind: KindELM327, name: "tcp", cands: []Candidate{{Info: Info{ID: "wifi-elm327"}}}}
	m := NewManager()
	m.RegisterDriver(NewMultiDriver(KindELM327, failing, healthy))

	cands, err := m.Discover(context.Background())
	if err == nil {
		t.Fatal("expected serial error to be preserved")
	}
	if len(cands) != 1 || cands[0].Info.ID != "wifi-elm327" {
		t.Fatalf("expected healthy TCP candidate, got %+v", cands)
	}
}

func TestNestedMultiDriverRouting(t *testing.T) {
	serial := &fakeDriver{kind: KindELM327, name: "serial", cands: []Candidate{{Info: Info{ID: "s"}}}}
	tcp := &fakeDriver{kind: KindELM327, name: "tcp", cands: []Candidate{{Info: Info{ID: "t"}}}}
	outer := NewMultiDriver(KindELM327, NewMultiDriver(KindELM327, serial, tcp))

	cands, err := outer.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		_, err := outer.Open(context.Background(), c)
		want := map[string]string{"s": "opened by serial", "t": "opened by tcp"}[c.Info.ID]
		if err == nil || err.Error() != want {
			t.Fatalf("candidate %s routed wrong: %v", c.Info.ID, err)
		}
	}
}
