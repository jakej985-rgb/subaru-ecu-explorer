package vehicle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const definitionsRoot = "../../data/definitions"

func TestLoadDefinitionsTargetSet(t *testing.T) {
	r := NewRegistry()
	report := LoadDefinitions(definitionsRoot, r)
	if !report.OK() {
		for _, res := range report.Results {
			if res.Err != nil {
				t.Errorf("%s: %v", res.Path, res.Err)
			}
		}
		t.Fatal("expected all shipped definitions to load")
	}
	if len(r.Profiles) != 1 || len(r.Engines) != 1 || len(r.Transmissions) != 1 ||
		len(r.Configurations) != 1 || len(r.Catalogs) != 1 {
		t.Fatalf("unexpected registry contents: %+v", r)
	}
	for _, cfg := range r.Configurations {
		if err := r.Resolve(cfg); err != nil {
			t.Fatalf("configuration %s references should resolve: %v", cfg.ID, err)
		}
	}
	for _, cat := range r.Catalogs {
		if err := r.ResolveCatalog(cat); err != nil {
			t.Fatalf("catalog %s references should resolve: %v", cat.ID, err)
		}
		if conflicts := cat.Conflicts(); len(conflicts) > 0 {
			t.Fatalf("shipped catalog should have no conflicts: %v", conflicts)
		}
	}
}

func TestLoadDefinitionsReportsBadFile(t *testing.T) {
	dir := t.TempDir()
	kindDir := filepath.Join(dir, "vehicles")
	if err := os.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := `{"schemaVersion":"1.0.0","id":"Bad ID","displayName":"","modelYear":{"status":"unknown"}}`
	if err := os.WriteFile(filepath.Join(kindDir, "bad.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	report := LoadDefinitions(dir, r)
	if report.OK() {
		t.Fatal("expected report to record the invalid profile")
	}
	if len(r.Profiles) != 0 {
		t.Fatal("invalid profile must not be registered")
	}
}

func TestStrictDecodeRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	kindDir := filepath.Join(dir, "vehicles")
	if err := os.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := `{"schemaVersion":"1.0.0","id":"p1","displayName":"P","surpriseField":42}`
	path := filepath.Join(kindDir, "extra.json")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	report := LoadDefinitions(dir, r)
	if report.OK() {
		t.Fatal("expected unknown top-level field to be rejected")
	}
}

// shippedDoc returns the first shipped definition in kindDir as a generic map.
func shippedDoc(t *testing.T, kindDir string) map[string]any {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(definitionsRoot, kindDir, "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no shipped %s definition found: %v", kindDir, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// loadSingle writes data as the only file in kindDir and loads it.
func loadSingle(t *testing.T, kindDir string, data []byte) LoadReport {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, kindDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, kindDir, "doc.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return LoadDefinitions(dir, NewRegistry())
}

func firstErr(report LoadReport) error {
	for _, res := range report.Results {
		if res.Err != nil {
			return res.Err
		}
	}
	return nil
}

func TestStrictDecodeRejectsUnknownFieldsInValidDocs(t *testing.T) {
	cases := []struct {
		name    string
		kindDir string
		mutate  func(map[string]any)
	}{
		{"profile top-level", "vehicles", func(d map[string]any) { d["surpriseField"] = 42 }},
		{"engine top-level", "engines", func(d map[string]any) { d["surpriseField"] = 42 }},
		{"catalog entry", "catalogs", func(d map[string]any) {
			entries, ok := d["entries"].([]any)
			if !ok || len(entries) == 0 {
				t.Fatal("shipped catalog has no entries")
			}
			entries[0].(map[string]any)["surpriseField"] = 42
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Sanity: the unmodified shipped doc loads cleanly on its own.
			orig, _ := json.Marshal(shippedDoc(t, tc.kindDir))
			if err := firstErr(loadSingle(t, tc.kindDir, orig)); err != nil {
				t.Fatalf("unmodified doc should load: %v", err)
			}
			doc := shippedDoc(t, tc.kindDir)
			tc.mutate(doc)
			data, _ := json.Marshal(doc)
			err := firstErr(loadSingle(t, tc.kindDir, data))
			if err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("expected unknown field error, got %v", err)
			}
		})
	}
}

func TestStrictDecodeRejectsTrailingContent(t *testing.T) {
	orig, _ := json.Marshal(shippedDoc(t, "vehicles"))
	for _, tail := range []string{" {}", " garbage"} {
		err := firstErr(loadSingle(t, "vehicles", append(append([]byte{}, orig...), tail...)))
		if err == nil || !strings.Contains(err.Error(), "trailing") {
			t.Fatalf("tail %q: expected trailing content error, got %v", tail, err)
		}
	}
}

func TestIncompleteProfileFailsValidation(t *testing.T) {
	// A profile missing required fact fields decodes to empty statuses and
	// must fail validation — unknowns must be explicit, not absent.
	var p VehicleProfile
	if err := json.Unmarshal([]byte(`{"schemaVersion":"1.0.0","id":"incomplete","displayName":"Incomplete"}`), &p); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected incomplete profile to fail validation")
	}
}

func TestConflictingAttributesDetected(t *testing.T) {
	entry := func(id, hw string) CatalogEntry {
		return CatalogEntry{
			ID:            id,
			Configuration: refFact("cfg-1"),
			Module:        ModuleEngineECU,
			Hardware:      Fact[string]{Value: &hw, Status: StatusConfirmed, Evidence: []Evidence{{Source: "s"}}},
			Tier:          TierConfirmed,
			Evidence:      []Evidence{{Source: "s"}},
		}
	}
	c := CompatibilityCatalog{
		SchemaVersion: CatalogSchemaVersion, ID: "cat", DisplayName: "Cat",
		Entries: []CatalogEntry{entry("e1", "hw-a"), entry("e2", "hw-b")},
	}
	conflicts := c.Conflicts()
	if len(conflicts) == 0 {
		t.Fatal("expected conflicting hardware references to be reported")
	}
	// Same hardware referenced twice is not a conflict.
	c.Entries[1] = entry("e2", "hw-a")
	if conflicts := c.Conflicts(); len(conflicts) != 0 {
		t.Fatalf("expected no conflicts for identical hardware, got %v", conflicts)
	}
}

func TestSchemaVersionEvolution(t *testing.T) {
	// Every shipped schema must pin the same schemaVersion the Go constants
	// enforce, so a version bump cannot drift between JSON and Go.
	checks := map[string]string{
		"vehicle-profile.schema.json":       CurrentSchemaVersion,
		"engine-config.schema.json":         ConfigSchemaVersion,
		"transmission-config.schema.json":   ConfigSchemaVersion,
		"drivetrain-config.schema.json":     ConfigSchemaVersion,
		"trim-config.schema.json":           ConfigSchemaVersion,
		"vehicle-configuration.schema.json": ConfigSchemaVersion,
		"hardware-profile.schema.json":      CatalogSchemaVersion,
		"hardware-catalog.schema.json":      CompatibilityCatalogSchemaVersion,
	}
	for file, want := range checks {
		data, err := os.ReadFile(filepath.Join("..", "..", "schemas", file))
		if err != nil {
			t.Fatalf("read schema %s: %v", file, err)
		}
		var schema struct {
			Properties struct {
				SchemaVersion struct {
					Const string `json:"const"`
				} `json:"schemaVersion"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatalf("parse schema %s: %v", file, err)
		}
		if schema.Properties.SchemaVersion.Const != want {
			t.Errorf("%s: schema const %q does not match Go version %q", file, schema.Properties.SchemaVersion.Const, want)
		}
	}

	// A document from another schema version must be rejected.
	p := validProfile()
	p.SchemaVersion = "0.9.0"
	if err := p.Validate(); err == nil {
		t.Fatal("expected schema version mismatch to fail")
	}
	p = validProfile()
	p.SchemaVersion = "2.0.0"
	if err := p.Validate(); err == nil {
		t.Fatal("expected future schema version to fail without migration")
	}
}

func TestUnknownFactValuesStayUnknown(t *testing.T) {
	// Unknown facts must not carry placeholder values.
	bad := `{"status":"unknown","value":"guess"}`
	var f Fact[string]
	if err := json.Unmarshal([]byte(bad), &f); err != nil {
		t.Fatal(err)
	}
	if err := validateFact("trim", f); err == nil {
		t.Fatal("expected unknown fact with value to fail")
	}
	// And confirmed facts without evidence fail too.
	good := `{"status":"confirmed","value":"x"}`
	if err := json.Unmarshal([]byte(good), &f); err != nil {
		t.Fatal(err)
	}
	err := validateFact("trim", f)
	if err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("expected evidence error, got %v", err)
	}
}
