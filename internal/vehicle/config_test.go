package vehicle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEngineConfigValidate(t *testing.T) {
	liters := 2.5
	c := EngineConfig{
		SchemaVersion:      ConfigSchemaVersion,
		ID:                 "subaru-ej25-2-5l",
		DisplayName:        "Subaru EJ25 2.5L",
		Make:               stringFact("Subaru"),
		Family:             stringFact("EJ25"),
		DisplacementLiters: Fact[float64]{Value: &liters, Status: StatusConfirmed, Evidence: []Evidence{{Source: "user-confirmed"}}},
		Cylinders:          Fact[int]{Status: StatusUnknown},
		Aspiration:         Fact[string]{Status: StatusUnknown},
		FuelSystem:         Fact[string]{Status: StatusUnknown},
		Valvetrain:         Fact[string]{Status: StatusUnknown},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected engine config to validate, got %v", err)
	}
}

func TestEngineConfigValidateRejectsBadValues(t *testing.T) {
	liters := 0.0
	cyl := 20
	c := EngineConfig{
		SchemaVersion:      ConfigSchemaVersion,
		ID:                 "bad-engine",
		DisplayName:        "Bad",
		Make:               stringFact("Subaru"),
		Family:             stringFact("EJ25"),
		DisplacementLiters: Fact[float64]{Value: &liters, Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}},
		Cylinders:          Fact[int]{Value: &cyl, Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected displacement/cylinder range errors")
	}
}

func TestTransmissionConfigValidate(t *testing.T) {
	c := TransmissionConfig{
		SchemaVersion: ConfigSchemaVersion,
		ID:            "manual-transmission-unverified",
		DisplayName:   "Manual transmission (variant unverified)",
		Type:          stringFact("manual"),
		Family:        Fact[string]{Status: StatusUnknown},
		GearCount:     Fact[int]{Status: StatusUnknown},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected transmission config to validate, got %v", err)
	}
}

func TestTransmissionConfigRejectsUnknownTypeValue(t *testing.T) {
	c := TransmissionConfig{
		SchemaVersion: ConfigSchemaVersion,
		ID:            "odd-transmission",
		DisplayName:   "Odd",
		Type:          stringFact("telepathic"),
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected enum violation for transmission type")
	}
}

func TestDrivetrainConfigValidate(t *testing.T) {
	c := DrivetrainConfig{
		SchemaVersion: ConfigSchemaVersion,
		ID:            "awd-drivetrain",
		DisplayName:   "AWD",
		Layout:        stringFact("awd"),
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected drivetrain config to validate, got %v", err)
	}
	c.Layout = stringFact("six-wheel-drive")
	if err := c.Validate(); err == nil {
		t.Fatal("expected enum violation for drivetrain layout")
	}
}

func TestTrimConfigValidate(t *testing.T) {
	c := TrimConfig{
		SchemaVersion: ConfigSchemaVersion,
		ID:            "usdm-legacy-outback-base",
		DisplayName:   "USDM Legacy Outback base",
		Market:        stringFact("USDM"),
		Model:         stringFact("Legacy Outback"),
		BodyCode:      Fact[string]{Status: StatusUnknown},
		BodyStyle:     Fact[string]{Status: StatusUnknown},
		TrimLevel:     Fact[string]{Status: StatusUnknown},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected trim config to validate, got %v", err)
	}
}

func TestVehicleConfigurationValidate(t *testing.T) {
	c := VehicleConfiguration{
		SchemaVersion: ConfigSchemaVersion,
		ID:            "subaru-legacy-outback-1999-usdm-ej25-manual",
		DisplayName:   "1999 USDM Legacy Outback EJ25 manual",
		Profile:       stringFact("subaru-legacy-outback-1999-usdm-ej25-manual"),
		Engine:        stringFact("subaru-ej25-2-5l"),
		Transmission:  stringFact("manual-transmission-unverified"),
		Drivetrain:    Fact[string]{Status: StatusUnknown},
		Body:          Fact[string]{Status: StatusUnknown},
		Trim:          Fact[string]{Status: StatusUnknown},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected configuration to validate, got %v", err)
	}
}

func TestRegistryResolve(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterProfile(validProfile()); err != nil {
		t.Fatal(err)
	}
	liters := 2.5
	if err := r.RegisterEngine(EngineConfig{
		SchemaVersion: ConfigSchemaVersion, ID: "subaru-ej25-2-5l", DisplayName: "EJ25",
		Make: stringFact("Subaru"), Family: stringFact("EJ25"),
		DisplacementLiters: Fact[float64]{Value: &liters, Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}},
		Cylinders:          Fact[int]{Status: StatusUnknown},
		Aspiration:         Fact[string]{Status: StatusUnknown},
		FuelSystem:         Fact[string]{Status: StatusUnknown},
		Valvetrain:         Fact[string]{Status: StatusUnknown},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterTransmission(TransmissionConfig{
		SchemaVersion: ConfigSchemaVersion, ID: "manual-transmission-unverified", DisplayName: "Manual",
		Type:      stringFact("manual"),
		Family:    Fact[string]{Status: StatusUnknown},
		GearCount: Fact[int]{Status: StatusUnknown},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := VehicleConfiguration{
		SchemaVersion: ConfigSchemaVersion, ID: "cfg-1", DisplayName: "Cfg 1",
		Profile:      stringFact("subaru-legacy-outback-1999-usdm-ej25-manual"),
		Engine:       stringFact("subaru-ej25-2-5l"),
		Transmission: stringFact("manual-transmission-unverified"),
		Drivetrain:   Fact[string]{Status: StatusUnknown},
		Body:         Fact[string]{Status: StatusUnknown},
		Trim:         Fact[string]{Status: StatusUnknown},
	}
	if err := r.Resolve(cfg); err != nil {
		t.Fatalf("expected all valued refs to resolve, got %v", err)
	}

	cfg.Engine = stringFact("nonexistent-engine")
	if err := r.Resolve(cfg); err == nil {
		t.Fatal("expected error for unregistered engine reference")
	}
}

func TestRegistryRejectsDuplicates(t *testing.T) {
	r := NewRegistry()
	p := validProfile()
	if err := r.RegisterProfile(p); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterProfile(p); err == nil {
		t.Fatal("expected duplicate profile id error")
	}
}

func loadDefinition(t *testing.T, relPath string, out any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode %s: %v", relPath, err)
	}
}

// TestTargetConfigurationDefinitions loads the shipped data definitions and
// verifies they validate and resolve as a consistent configuration.
func TestTargetConfigurationDefinitions(t *testing.T) {
	r := NewRegistry()

	var profile VehicleProfile
	loadDefinition(t, filepath.Join("data", "definitions", "vehicles", "1999-usdm-legacy-outback-ej25-manual.json"), &profile)
	if err := r.RegisterProfile(profile); err != nil {
		t.Fatalf("profile: %v", err)
	}

	var engine EngineConfig
	loadDefinition(t, filepath.Join("data", "definitions", "engines", "subaru-ej25-2-5l.json"), &engine)
	if err := r.RegisterEngine(engine); err != nil {
		t.Fatalf("engine: %v", err)
	}

	var transmission TransmissionConfig
	loadDefinition(t, filepath.Join("data", "definitions", "transmissions", "manual-transmission-unverified.json"), &transmission)
	if err := r.RegisterTransmission(transmission); err != nil {
		t.Fatalf("transmission: %v", err)
	}

	var cfg VehicleConfiguration
	loadDefinition(t, filepath.Join("data", "definitions", "configurations", "1999-usdm-legacy-outback-ej25-manual.json"), &cfg)
	if err := r.RegisterConfiguration(cfg); err != nil {
		t.Fatalf("configuration: %v", err)
	}

	if err := r.Resolve(cfg); err != nil {
		t.Fatalf("target configuration references should resolve: %v", err)
	}

	// Unverified components must remain explicitly unknown.
	for name, fact := range map[string]Fact[string]{
		"drivetrain": cfg.Drivetrain, "body": cfg.Body, "trim": cfg.Trim,
	} {
		if fact.Status != StatusUnknown || fact.Value != nil {
			t.Errorf("%s: expected explicit unknown with no value, got %+v", name, fact)
		}
	}
}
