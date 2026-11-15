package vehicle

import (
	"strings"
	"testing"
)

func stdFact(v string) Fact[string] {
	return Fact[string]{
		Value:    &v,
		Status:   StatusConfirmed,
		Evidence: []Evidence{{Source: "SAE J1962"}},
	}
}

func validConnector() Connector {
	return Connector{
		SchemaVersion: ConnectorSchemaVersion,
		ID:            "test-dlc",
		DisplayName:   "Test DLC",
		Kind:          ConnectorOBDIIDLC,
		PinCount:      2,
		Pins: []ConnectorPin{
			{Number: 1, Function: stdFact("power")},
			{Number: 2, Function: Fact[string]{Status: StatusUnknown}},
		},
	}
}

func TestConnectorValidation(t *testing.T) {
	if err := validConnector().Validate(); err != nil {
		t.Fatalf("valid connector rejected: %v", err)
	}

	bad := validConnector()
	bad.SchemaVersion = "0.9.0"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected schema version mismatch to fail")
	}

	bad = validConnector()
	bad.Pins[0].Number = 3 // exceeds pinCount
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range pin error, got %v", err)
	}

	bad = validConnector()
	bad.Pins[1].Number = 1 // duplicate
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate pin error, got %v", err)
	}

	bad = validConnector()
	bad.Kind = "obd-ii-dlc-extra"
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind error, got %v", err)
	}

	// Unknown pin functions stay unknown: no value, no evidence required.
	bad = validConnector()
	bad.Pins[1].Function = Fact[string]{Status: StatusUnknown, Value: strPtr("guessed")}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected unknown fact carrying a value to fail")
	}
}

func strPtr(s string) *string { return &s }

func TestShippedDLCDefinition(t *testing.T) {
	r := NewRegistry()
	report := LoadDefinitions("../../data/definitions", r)
	if !report.OK() {
		for _, res := range report.Results {
			if res.Err != nil {
				t.Fatalf("%s: %v", res.Path, res.Err)
			}
		}
	}
	c, ok := r.Connectors["1999-usdm-legacy-outback-dlc"]
	if !ok {
		t.Fatal("shipped DLC connector definition not registered")
	}
	if c.PinCount != 16 || len(c.Pins) != 16 {
		t.Fatalf("expected 16 documented pins, got pinCount=%d pins=%d", c.PinCount, len(c.Pins))
	}

	byNumber := map[int]ConnectorPin{}
	for _, p := range c.Pins {
		byNumber[p.Number] = p
	}

	// Ground positions must be documented with standard evidence.
	for _, n := range []int{4, 5} {
		p := byNumber[n]
		if p.Function.Value == nil || *p.Function.Value != "ground" {
			t.Fatalf("pin %d: expected ground function, got %+v", n, p.Function)
		}
		if len(p.Function.Evidence) == 0 {
			t.Fatalf("pin %d: ground claim lacks evidence", n)
		}
	}
	// Pin 16 is the unswitched +12V supply.
	p16 := byNumber[16]
	if p16.Function.Value == nil || *p16.Function.Value != "battery-12v-unswitched" {
		t.Fatalf("pin 16: unexpected function %+v", p16.Function)
	}
	if len(p16.Function.Evidence) == 0 {
		t.Fatal("pin 16: power claim lacks evidence")
	}

	// ISO 9141 K/L lines are the expected OBD-II data path for this era.
	for n, want := range map[int]string{7: "iso9141-k-line", 15: "iso9141-l-line"} {
		if got := *byNumber[n].Function.Value; got != want {
			t.Fatalf("pin %d: expected %q, got %q", n, want, got)
		}
	}

	// Manufacturer-discretionary pins must not claim vehicle-specific
	// functions — SSM terminal assignments stay unverified.
	for _, n := range []int{1, 3, 8, 9, 11, 12, 13} {
		if got := *byNumber[n].Function.Value; got != "manufacturer-discretionary" {
			t.Fatalf("pin %d: expected manufacturer-discretionary, got %q", n, got)
		}
	}
}
