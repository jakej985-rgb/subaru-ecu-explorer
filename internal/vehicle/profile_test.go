package vehicle

import (
	"encoding/json"
	"testing"
)

func stringFact(value string) Fact[string] {
	return Fact[string]{Value: &value, Status: StatusConfirmed, Evidence: []Evidence{{Source: "user-confirmed"}}}
}

func TestValidateAcceptsEvidenceBackedProfile(t *testing.T) {
	year := 1999
	p := VehicleProfile{
		SchemaVersion: CurrentSchemaVersion,
		ID:            "subaru-legacy-outback-1999-usdm-ej25-manual",
		DisplayName:   "1999 USDM Subaru Legacy Outback 2.5L EJ25 Manual",
		ModelYear:     Fact[int]{Value: &year, Status: StatusConfirmed, Evidence: []Evidence{{Source: "user-confirmed"}}},
		Market:        stringFact("USDM"),
		Make:          stringFact("Subaru"),
		Model:         stringFact("Legacy Outback"),
		Body:          Fact[string]{Status: StatusUnknown},
		Trim:          Fact[string]{Status: StatusUnknown},
		Engine:        stringFact("EJ25 2.5L"),
		Transmission:  stringFact("Manual"),
		Drivetrain:    Fact[string]{Status: StatusUnknown},
		Identifiers: map[string]Fact[string]{
			"engineECUPartNumber": {Status: StatusUnknown},
			"absModulePartNumber": {Status: StatusUnknown},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected profile to validate, got %v", err)
	}
}

func TestValidateRejectsConfirmedFactWithoutEvidence(t *testing.T) {
	value := "USDM"
	p := Fact[string]{Value: &value, Status: StatusConfirmed}
	if err := validateFact("market", p); err == nil {
		t.Fatal("expected evidence validation error")
	}
}

func TestValidateRejectsUnknownFactWithValue(t *testing.T) {
	value := "unknown-but-guessed"
	p := Fact[string]{Value: &value, Status: StatusUnknown}
	if err := validateFact("trim", p); err == nil {
		t.Fatal("expected unknown value validation error")
	}
}

func TestValidateRejectsUnsupportedStatusWithValue(t *testing.T) {
	value := "manual"
	p := Fact[string]{Value: &value, Status: StatusUnsupported}
	if err := validateFact("transmission", p); err == nil {
		t.Fatal("expected unsupported value validation error")
	}
}

func validProfile() VehicleProfile {
	year := 1999
	return VehicleProfile{
		SchemaVersion: CurrentSchemaVersion,
		ID:            "subaru-legacy-outback-1999-usdm-ej25-manual",
		DisplayName:   "1999 USDM Subaru Legacy Outback",
		ModelYear:     Fact[int]{Value: &year, Status: StatusConfirmed, Evidence: []Evidence{{Source: "user-confirmed"}}},
		Market:        stringFact("USDM"),
		Make:          stringFact("Subaru"),
		Model:         stringFact("Legacy Outback"),
		Body:          Fact[string]{Status: StatusUnknown},
		Trim:          Fact[string]{Status: StatusUnknown},
		Engine:        stringFact("EJ25 2.5L"),
		Transmission:  stringFact("Manual"),
		Drivetrain:    Fact[string]{Status: StatusUnknown},
	}
}

func TestValidateProduction(t *testing.T) {
	p := validProfile()
	if err := p.Validate(); err != nil {
		t.Fatalf("omitted production should validate, got %v", err)
	}
	p.Production = &Fact[ProductionRange]{Status: StatusUnknown}
	if err := p.Validate(); err != nil {
		t.Fatalf("unknown production should validate, got %v", err)
	}
	p.Production = &Fact[ProductionRange]{}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for present production with empty status")
	}
}

func TestValidateRejectsBadIDs(t *testing.T) {
	for _, id := range []string{"Legacy", "a_b", "a b", "-a", "a-", "a--b"} {
		p := validProfile()
		p.ID = id
		if err := p.Validate(); err == nil {
			t.Errorf("expected error for id %q", id)
		}
	}
}

func TestValidateValueConstraints(t *testing.T) {
	p := validProfile()
	p.Market = stringFact("")
	if err := p.Validate(); err == nil {
		t.Error("expected error for empty string value")
	}
	for _, y := range []int{1899, 2101} {
		p := validProfile()
		year := y
		p.ModelYear.Value = &year
		if err := p.Validate(); err == nil {
			t.Errorf("expected error for year %d", y)
		}
	}
	for _, y := range []int{1900, 2100} {
		p := validProfile()
		year := y
		p.ModelYear.Value = &year
		if err := p.Validate(); err != nil {
			t.Errorf("year %d should validate, got %v", y, err)
		}
	}
}

func TestDecodeProductionNull(t *testing.T) {
	base, err := json.Marshal(validProfile())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		key        string
		production json.RawMessage
		wantErr    bool
	}{
		"omitted":         {"production", nil, false},
		"null":            {"production", json.RawMessage("null"), true},
		"capitalizedNull": {"Production", json.RawMessage("null"), true},
		"object":          {"production", json.RawMessage(`{"status":"unknown"}`), false},
	}
	for name, tc := range cases {
		delete(m, "production")
		delete(m, "Production")
		if tc.production != nil {
			m[tc.key] = tc.production
		}
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var p VehicleProfile
		err = json.Unmarshal(data, &p)
		if err == nil {
			err = p.Validate()
		}
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: wantErr=%v, got %v", name, tc.wantErr, err)
		}
	}
}

func TestDecodeIntoPopulatedProfilePreservesFields(t *testing.T) {
	p := validProfile()
	if err := json.Unmarshal([]byte(`{"production":{"status":"unknown"}}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.Production == nil || p.Production.Status != StatusUnknown {
		t.Fatalf("expected production to be decoded, got %+v", p.Production)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("partial decode should preserve existing fields, got %v", err)
	}
}

func TestValidateRejectsInvalidStatus(t *testing.T) {
	p := Fact[string]{Status: "maybe"}
	if err := validateFact("engine", p); err == nil {
		t.Fatal("expected invalid status error")
	}
}
