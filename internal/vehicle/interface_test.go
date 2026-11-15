package vehicle

import (
	"strings"
	"testing"
)

func TestShippedInterfaceDefinitions(t *testing.T) {
	r := NewRegistry()
	report := LoadDefinitions("../../data/definitions", r)
	if !report.OK() {
		for _, res := range report.Results {
			if res.Err != nil {
				t.Fatalf("%s: %v", res.Path, res.Err)
			}
		}
	}

	// The generic ELM327 path must never claim SSM protocol support.
	elm, ok := r.Interfaces["elm327-odbii"]
	if !ok {
		t.Fatal("elm327-odbii interface definition missing")
	}
	if elm.Protocol.Value == nil || *elm.Protocol.Value != "generic-obd-ii" {
		t.Fatalf("ELM327 protocol must stay generic-obd-ii, got %+v", elm.Protocol)
	}

	// The SSM path profile must exist and stay read-only.
	ssm, ok := r.Interfaces["subaru-ssm"]
	if !ok {
		t.Fatal("subaru-ssm interface definition missing")
	}
	if ssm.ReadOnly.Value == nil || !*ssm.ReadOnly.Value {
		t.Fatal("SSM interface must declare read-only capability")
	}

	// Candidate SSM hardware must be inferred, never confirmed, for the
	// vehicle-specific protocol claim.
	for _, id := range []string{"tactrix-openport-13", "vag-com-kkl-ftdi-serial"} {
		p, ok := r.Interfaces[id]
		if !ok {
			t.Fatalf("candidate interface %s missing", id)
		}
		if p.Kind != InterfaceSubaruSSM {
			t.Fatalf("%s: expected kind subaru-ssm, got %s", id, p.Kind)
		}
		if p.Protocol.Status == StatusConfirmed {
			t.Fatalf("%s: vehicle SSM compatibility must not be confirmed without vehicle evidence", id)
		}
		if p.Protocol.Value == nil || !strings.Contains(*p.Protocol.Value, "ssm") {
			t.Fatalf("%s: expected ssm protocol value, got %+v", id, p.Protocol)
		}
	}
}
