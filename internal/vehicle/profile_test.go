package vehicle

import "testing"

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

func TestValidateRejectsInvalidStatus(t *testing.T) {
	p := Fact[string]{Status: "maybe"}
	if err := validateFact("engine", p); err == nil {
		t.Fatal("expected invalid status error")
	}
}
