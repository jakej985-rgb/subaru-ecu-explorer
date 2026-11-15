package vehicle

import (
	"testing"
)

// testRegistry builds a registry with one profile, one configuration, and the
// component configs needed for resolver tests.
func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	year := 1999
	p := VehicleProfile{
		SchemaVersion: CurrentSchemaVersion, ID: "veh-1", DisplayName: "Test Vehicle",
		ModelYear: Fact[int]{Value: &year, Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}},
		Market:    stringFact("USDM"), Make: stringFact("Subaru"), Model: stringFact("Legacy Outback"),
		Body: Fact[string]{Status: StatusUnknown}, Trim: Fact[string]{Status: StatusUnknown},
		Engine: stringFact("EJ25 2.5L"), Transmission: stringFact("Manual"), Drivetrain: Fact[string]{Status: StatusUnknown},
	}
	if err := r.RegisterProfile(p); err != nil {
		t.Fatal(err)
	}
	liters := 2.5
	eng := EngineConfig{
		SchemaVersion: ConfigSchemaVersion, ID: "eng-ej25", DisplayName: "EJ25",
		Make: stringFact("Subaru"), Family: stringFact("EJ25"),
		DisplacementLiters: Fact[float64]{Value: &liters, Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}},
		Cylinders:          Fact[int]{Status: StatusUnknown}, Aspiration: Fact[string]{Status: StatusUnknown},
		FuelSystem: Fact[string]{Status: StatusUnknown}, Valvetrain: Fact[string]{Status: StatusUnknown},
	}
	if err := r.RegisterEngine(eng); err != nil {
		t.Fatal(err)
	}
	tr := TransmissionConfig{
		SchemaVersion: ConfigSchemaVersion, ID: "trn-manual", DisplayName: "Manual",
		Type: stringFact("manual"), Family: Fact[string]{Status: StatusUnknown}, GearCount: Fact[int]{Status: StatusUnknown},
	}
	if err := r.RegisterTransmission(tr); err != nil {
		t.Fatal(err)
	}
	cfg := VehicleConfiguration{
		SchemaVersion: ConfigSchemaVersion, ID: "cfg-1", DisplayName: "Cfg",
		Profile: refFact("veh-1"), Engine: refFact("eng-ej25"), Transmission: refFact("trn-manual"),
		Drivetrain: Fact[string]{Status: StatusUnknown}, Body: Fact[string]{Status: StatusUnknown}, Trim: Fact[string]{Status: StatusUnknown},
	}
	if err := r.RegisterConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	return r
}

func constraintFactIntRange(min, max int) *Fact[IntRange] {
	r := IntRange{Min: min, Max: max}
	return &Fact[IntRange]{Value: &r, Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}}
}

func constraintFactString(v string) *Fact[string] {
	return &Fact[string]{Value: &v, Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}}
}

func catalogEntry(id string, module ModuleKind, tier EvidenceTier, hw string, app *Applicability) CatalogEntry {
	e := CatalogEntry{
		ID: id, Configuration: refFact("cfg-1"), Module: module, Tier: tier, Applicability: app,
		Hardware: unknownFact(),
	}
	if hw != "" {
		e.Hardware = refFact(hw)
		e.Evidence = []Evidence{{Source: "catalog-src"}}
	}
	return e
}

func TestResolveFitmentExactMatch(t *testing.T) {
	r := testRegistry(t)
	cat := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{
			catalogEntry("e1", ModuleEngineECU, TierCandidate, "hw-ecu-a", &Applicability{
				ModelYear:    constraintFactIntRange(1998, 2000),
				Market:       constraintFactString("USDM"),
				Engine:       constraintFactString("eng-ej25"),
				Transmission: constraintFactString("trn-manual"),
			}),
		},
	}
	rep, err := r.ResolveFitment(cat, "cfg-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Outcome != FitCompatibleCandidate {
		t.Fatalf("expected compatible candidate, got %+v", rep.Results)
	}
	if len(rep.Compatible) != 1 {
		t.Fatal("expected one compatible entry")
	}
	for _, c := range rep.Results[0].Constraints {
		if c.State == ConstraintNotSupplied {
			continue
		}
		if c.State != ConstraintMatch {
			t.Errorf("constraint %s: expected match, got %s (%s)", c.Dimension, c.State, c.Detail)
		}
	}
}

func TestResolveFitmentKnownConflictBeatsTier(t *testing.T) {
	r := testRegistry(t)
	// Confirmed-tier entry whose engine constraint conflicts must be
	// incompatible — tier cannot override a known applicability conflict.
	cat := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{
			catalogEntry("e1", ModuleEngineECU, TierConfirmed, "hw-wrong", &Applicability{
				Engine: constraintFactString("eng-other"),
			}),
		},
	}
	rep, err := r.ResolveFitment(cat, "cfg-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	res := rep.Results[0]
	if res.Outcome != FitIncompatible {
		t.Fatalf("expected incompatible despite confirmed tier, got %s", res.Outcome)
	}
	if len(rep.Compatible) != 0 {
		t.Fatal("conflicting entry must not appear in compatible list")
	}
}

func TestResolveFitmentMissingFactsInsufficient(t *testing.T) {
	r := testRegistry(t)
	// Constrain drivetrain — the vehicle's drivetrain ref is unknown.
	cat := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{
			catalogEntry("e1", ModuleDrivetrainProbe(), TierCandidate, "hw-x", &Applicability{
				Drivetrain: constraintFactString("awd"),
			}),
		},
	}
	rep, err := r.ResolveFitment(cat, "cfg-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	res := rep.Results[0]
	if res.Outcome != FitInsufficientInfo {
		t.Fatalf("expected insufficient-information, got %s", res.Outcome)
	}
	var sawUnknown bool
	for _, c := range res.Constraints {
		if c.Dimension == "drivetrain" && c.State == ConstraintUnknown {
			sawUnknown = true
		}
	}
	if !sawUnknown {
		t.Fatal("expected drivetrain constraint to evaluate as unknown")
	}
}

func ModuleDrivetrainProbe() ModuleKind { return ModuleOther }

func TestResolveFitmentPartialVINYearAmbiguity(t *testing.T) {
	r := testRegistry(t)
	// Vehicle profile year is 1999 confirmed, so year is known — to exercise
	// ambiguity use a profile-free path: remove profile's year influence by
	// testing with an identification whose VIN has two candidates and a
	// configuration whose profile fact is unknown.
	cfg := VehicleConfiguration{
		SchemaVersion: ConfigSchemaVersion, ID: "cfg-partial", DisplayName: "Partial",
		Profile: refFact("veh-1"), Engine: refFact("eng-ej25"), Transmission: refFact("trn-manual"),
		Drivetrain: Fact[string]{Status: StatusUnknown}, Body: Fact[string]{Status: StatusUnknown}, Trim: Fact[string]{Status: StatusUnknown},
	}
	if err := r.RegisterConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	// Strip the profile year to simulate unresolved identification.
	p := r.Profiles["veh-1"]
	p.ModelYear = Fact[int]{Status: StatusUnknown}
	r.Profiles["veh-1"] = p

	id := &VehicleIdentification{VINDecode: &VINDecode{YearCandidates: []int{1999, 2029}}}
	cat := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{
			catalogEntry("e1", ModuleEngineECU, TierCandidate, "hw-x", &Applicability{
				ModelYear: constraintFactIntRange(1995, 2005),
			}),
		},
	}
	// Entry targets cfg-1 though; make it target cfg-partial.
	cat.Entries[0].Configuration = refFact("cfg-partial")
	rep, err := r.ResolveFitment(cat, "cfg-partial", id)
	if err != nil {
		t.Fatal(err)
	}
	res := rep.Results[0]
	if res.Outcome != FitInsufficientInfo {
		t.Fatalf("ambiguous VIN year must yield insufficient-information, got %s", res.Outcome)
	}
	if len(rep.Vehicle.YearCandidates) != 2 {
		t.Fatalf("expected year candidates preserved, got %v", rep.Vehicle.YearCandidates)
	}
}

func TestResolveFitmentMultipleCandidatesPreserved(t *testing.T) {
	r := testRegistry(t)
	cat := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{
			catalogEntry("e-cand", ModuleEngineECU, TierCandidate, "hw-cand", nil),
			catalogEntry("e-conf", ModuleEngineECU, TierConfirmed, "hw-conf", nil),
			catalogEntry("e-corr", ModuleEngineECU, TierCorroborated, "hw-corr", nil),
		},
	}
	// Corroborated needs two distinct sources to validate — bypass Validate by
	// constructing the catalog directly (resolver does not re-validate).
	rep, err := r.ResolveFitment(cat, "cfg-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Compatible) != 3 {
		t.Fatalf("all three plausible matches must be preserved, got %d", len(rep.Compatible))
	}
	// Deterministic ranking: confirmed, corroborated, candidate.
	want := []string{"e-conf", "e-corr", "e-cand"}
	for i, res := range rep.Compatible {
		if res.EntryID != want[i] {
			t.Fatalf("rank %d: expected %s, got %s", i, want[i], res.EntryID)
		}
	}
}

func TestResolveFitmentConflictingEvidence(t *testing.T) {
	r := testRegistry(t)
	e := catalogEntry("e1", ModuleEngineECU, TierCandidate, "hw-x", nil)
	e.Conflicts = []ConflictRecord{{
		Attribute: "partNumber",
		Claims: []ConflictClaim{
			{Value: "123", Source: "source-a"},
			{Value: "456", Source: "source-b"},
		},
		Status: ConflictOpen,
	}}
	cat := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{e},
	}
	rep, err := r.ResolveFitment(cat, "cfg-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Results[0].Outcome != FitConflictingEvidence {
		t.Fatalf("expected conflicting-evidence, got %s", rep.Results[0].Outcome)
	}
}

func TestResolveFitmentABSSeparation(t *testing.T) {
	r := testRegistry(t)
	// ABS controller and hydraulic assembly are distinct module identities.
	cat := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{
			catalogEntry("abs-ctrl", ModuleABSController, TierUnresolved, "", nil),
			catalogEntry("abs-hyd", ModuleABSHydraulicAssembly, TierUnresolved, "", nil),
		},
	}
	rep, err := r.ResolveFitment(cat, "cfg-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 2 {
		t.Fatalf("expected separate results per module identity, got %d", len(rep.Results))
	}
	modules := map[ModuleKind]bool{}
	for _, res := range rep.Results {
		modules[res.Module] = true
	}
	if !modules[ModuleABSController] || !modules[ModuleABSHydraulicAssembly] {
		t.Fatalf("expected distinct ABS controller and hydraulic assembly results, got %v", modules)
	}
}

func TestResolveFitmentDeterministic(t *testing.T) {
	r := testRegistry(t)
	entries := []CatalogEntry{
		catalogEntry("e-b", ModuleEngineECU, TierCandidate, "hw-b", nil),
		catalogEntry("e-a", ModuleEngineECU, TierCandidate, "hw-a", nil),
	}
	cat := CompatibilityCatalog{SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat", Entries: entries}
	rep1, _ := r.ResolveFitment(cat, "cfg-1", nil)
	rep2, _ := r.ResolveFitment(cat, "cfg-1", nil)
	if len(rep1.Results) != 2 || rep1.Results[0].EntryID != "e-a" || rep1.Results[1].EntryID != "e-b" {
		t.Fatalf("expected deterministic entry-ID ordering, got %+v", rep1.Results)
	}
	for i := range rep1.Results {
		if rep1.Results[i].EntryID != rep2.Results[i].EntryID {
			t.Fatal("resolver output not deterministic across runs")
		}
	}
}

func TestInterfaceProfilesDoNotEnterFitment(t *testing.T) {
	r := testRegistry(t)
	// Register interface profiles; they must not appear in fitment results.
	for _, p := range []InterfaceProfile{
		{SchemaVersion: InterfaceSchemaVersion, ID: "elm327-odbii", DisplayName: "ELM327", Kind: InterfaceELM327,
			Transport: stringFact("obd-ii"), Protocol: stringFact("generic-obd-ii"),
			ReadOnly: Fact[bool]{Value: boolPtr(true), Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}}},
	} {
		if err := r.RegisterInterface(p); err != nil {
			t.Fatal(err)
		}
	}
	cat := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{catalogEntry("e1", ModuleEngineECU, TierCandidate, "hw-x", nil)},
	}
	rep, err := r.ResolveFitment(cat, "cfg-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 1 {
		t.Fatalf("interface profiles must not appear in fitment results, got %d", len(rep.Results))
	}
}

func boolPtr(b bool) *bool { return &b }

// TestResolveFitmentShippedCatalog verifies the real target catalog resolves
// every entry as unresolved — no target hardware is promoted without evidence.
func TestResolveFitmentShippedCatalog(t *testing.T) {
	r := NewRegistry()
	report := LoadDefinitions("../../data/definitions", r)
	if !report.OK() {
		t.Fatal("shipped definitions must load")
	}
	const cfgID = "subaru-legacy-outback-1999-usdm-ej25-manual"
	var cat CompatibilityCatalog
	for _, c := range r.Catalogs {
		cat = c
	}
	rep, err := r.ResolveFitment(cat, cfgID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) == 0 {
		t.Fatal("expected catalog entries for target configuration")
	}
	for _, res := range rep.Results {
		if res.Outcome != FitUnresolved {
			t.Errorf("entry %s (%s): expected unresolved, got %s", res.EntryID, res.Module, res.Outcome)
		}
	}
	if len(rep.Compatible) != 0 {
		t.Fatalf("no target hardware may be promoted: %+v", rep.Compatible)
	}
}
