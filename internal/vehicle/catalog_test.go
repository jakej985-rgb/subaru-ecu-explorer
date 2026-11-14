package vehicle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func refFact(v string) Fact[string] {
	return Fact[string]{Value: &v, Status: StatusConfirmed, Evidence: []Evidence{{Source: "test"}}}
}

func unknownFact() Fact[string] { return Fact[string]{Status: StatusUnknown} }

func validEntry() CatalogEntry {
	return CatalogEntry{
		ID:            "engine-ecu",
		Configuration: refFact("cfg-1"),
		Module:        ModuleEngineECU,
		Hardware:      unknownFact(),
		Tier:          TierUnresolved,
	}
}

func TestCatalogEntryValidateUnresolved(t *testing.T) {
	if err := validEntry().Validate(); err != nil {
		t.Fatalf("expected unresolved entry to validate, got %v", err)
	}
}

func TestCatalogEntryTierRules(t *testing.T) {
	// Valued tiers require hardware + evidence.
	e := validEntry()
	e.Tier = TierCandidate
	if err := e.Validate(); err == nil {
		t.Fatal("expected candidate tier without hardware/evidence to fail")
	}
	e.Hardware = refFact("hw-1")
	e.Evidence = []Evidence{{Source: "service-manual"}}
	if err := e.Validate(); err != nil {
		t.Fatalf("expected candidate entry to validate, got %v", err)
	}

	// Unresolved must not assert hardware.
	e = validEntry()
	e.Hardware = refFact("hw-1")
	if err := e.Validate(); err == nil {
		t.Fatal("expected unresolved entry with hardware to fail")
	}

	// Confirmed and corroborated follow the same rules.
	for _, tier := range []EvidenceTier{TierConfirmed, TierCorroborated} {
		e = validEntry()
		e.Tier = tier
		e.Hardware = refFact("hw-1")
		e.Evidence = []Evidence{{Source: "s"}}
		if err := e.Validate(); err != nil {
			t.Errorf("tier %s should validate, got %v", tier, err)
		}
	}
}

func TestCatalogValidateDuplicateEntries(t *testing.T) {
	c := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion,
		ID:            "cat-1",
		DisplayName:   "Cat",
		Entries:       []CatalogEntry{validEntry(), validEntry()},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected duplicate entry id error")
	}
}

func TestCatalogCandidatesOrdering(t *testing.T) {
	mk := func(id string, tier EvidenceTier, hw string) CatalogEntry {
		e := validEntry()
		e.ID = id
		e.Tier = tier
		if hw != "" {
			e.Hardware = refFact(hw)
			e.Evidence = []Evidence{{Source: "s"}}
		}
		return e
	}
	c := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{
			mk("cand", TierCandidate, "hw-cand"),
			mk("unres", TierUnresolved, ""),
			mk("conf", TierConfirmed, "hw-conf"),
			mk("corr", TierCorroborated, "hw-corr"),
			{ID: "other-cfg", Configuration: refFact("cfg-2"), Module: ModuleEngineECU, Hardware: unknownFact(), Tier: TierUnresolved},
			{ID: "other-mod", Configuration: refFact("cfg-1"), Module: ModuleABSController, Hardware: unknownFact(), Tier: TierUnresolved},
		},
	}
	got := c.CatalogCandidates("cfg-1", ModuleEngineECU)
	if len(got) != 4 {
		t.Fatalf("expected 4 entries for cfg-1/engine-ecu, got %d", len(got))
	}
	wantOrder := []EvidenceTier{TierConfirmed, TierCorroborated, TierCandidate, TierUnresolved}
	for i, e := range got {
		if e.Tier != wantOrder[i] {
			t.Fatalf("position %d: expected tier %s, got %s", i, wantOrder[i], e.Tier)
		}
	}
}

func TestResolveCatalog(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterConfiguration(VehicleConfiguration{
		SchemaVersion: ConfigSchemaVersion, ID: "vc-1", DisplayName: "VC",
		Profile:      unknownFact(),
		Engine:       unknownFact(),
		Transmission: unknownFact(),
		Drivetrain:   unknownFact(),
		Body:         unknownFact(),
		Trim:         unknownFact(),
	}); err != nil {
		t.Fatal(err)
	}

	// Unresolved entry referencing an unregistered configuration must fail.
	bad := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat-bad", DisplayName: "Bad",
		Entries: []CatalogEntry{{
			ID: "e1", Configuration: refFact("missing-cfg"), Module: ModuleEngineECU,
			Hardware: unknownFact(), Tier: TierUnresolved,
		}},
	}
	if err := r.ResolveCatalog(bad); err == nil {
		t.Fatal("expected unresolved configuration reference error")
	}

	// Candidate entry referencing an unregistered hardware profile must fail.
	bad.Entries[0].Configuration = refFact("vc-1")
	bad.Entries[0].Tier = TierCandidate
	bad.Entries[0].Hardware = refFact("missing-hw")
	bad.Entries[0].Evidence = []Evidence{{Source: "s"}}
	if err := r.ResolveCatalog(bad); err == nil {
		t.Fatal("expected unresolved hardware reference error")
	}
}

func loadCatalogFixture(t *testing.T) CompatibilityCatalog {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "data", "definitions", "catalogs", "1999-usdm-legacy-outback-ej25-manual.json"))
	if err != nil {
		t.Fatalf("read catalog fixture: %v", err)
	}
	var c CompatibilityCatalog
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	return c
}

func loadConfigurationFixture(t *testing.T) VehicleConfiguration {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "data", "definitions", "configurations", "1999-usdm-legacy-outback-ej25-manual.json"))
	if err != nil {
		t.Fatalf("read configuration fixture: %v", err)
	}
	var c VehicleConfiguration
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("decode configuration: %v", err)
	}
	return c
}

// TestTargetCatalog verifies the shipped catalog validates, resolves against
// the shipped configuration, and asserts no hardware as installed.
func TestTargetCatalog(t *testing.T) {
	r := NewRegistry()
	cfg := loadConfigurationFixture(t)
	if err := r.RegisterConfiguration(cfg); err != nil {
		t.Fatalf("register configuration: %v", err)
	}
	cat := loadCatalogFixture(t)
	if err := r.RegisterCatalog(cat); err != nil {
		t.Fatalf("catalog validation: %v", err)
	}
	if err := r.ResolveCatalog(cat); err != nil {
		t.Fatalf("catalog references should resolve: %v", err)
	}

	for _, module := range []ModuleKind{ModuleEngineECU, ModuleABSController, ModuleTransmissionController} {
		entries := cat.CatalogCandidates(cfg.ID, module)
		if len(entries) != 1 || entries[0].Tier != TierUnresolved {
			t.Errorf("module %s: expected one unresolved entry, got %+v", module, entries)
		}
	}

	// All shipped entries must be unresolved: nothing asserts installed hardware.
	for _, e := range cat.Entries {
		if e.Tier != TierUnresolved {
			t.Errorf("entry %s: expected unresolved tier for target vehicle, got %s", e.ID, e.Tier)
		}
		if e.Hardware.Value != nil {
			t.Errorf("entry %s: unresolved entry must not name hardware", e.ID)
		}
	}
}
