package vehicle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// CatalogSchemaVersion versions the hardware profile document format.
const CatalogSchemaVersion = "1.0.0"

// CompatibilityCatalogSchemaVersion versions the compatibility catalog
// document format. 2.0.0 replaced the production-range applicability fact
// with structured Applicability and free-text conflicts with ConflictRecord;
// 1.0.0 catalogs must be migrated (see docs/HARDWARE_CATALOG.md).
const CompatibilityCatalogSchemaVersion = "2.0.0"

// ModuleKind identifies the vehicle module a catalog entry applies to.
type ModuleKind string

const (
	ModuleEngineECU     ModuleKind = "engine-ecu"
	ModuleABSController ModuleKind = "abs-controller"
	// ModuleABSHydraulicAssembly is the ABS hydraulic/modulator unit, a
	// separate hardware identity from the electronic ABS controller.
	ModuleABSHydraulicAssembly   ModuleKind = "abs-hydraulic-assembly"
	ModuleTransmissionController ModuleKind = "transmission-controller"
	ModuleOther                  ModuleKind = "other-module"
)

// EvidenceTier ranks how strongly a catalog entry supports a hardware
// candidate for a configuration. It is deliberately separate from FactStatus:
// a catalog entry describes compatibility evidence, never an installed part.
type EvidenceTier string

const (
	// TierConfirmed: directly confirmed for this configuration by evidence.
	TierConfirmed EvidenceTier = "confirmed"
	// TierCorroborated: multiple independent sources agree.
	TierCorroborated EvidenceTier = "corroborated"
	// TierCandidate: plausible but unverified; never presented as installed.
	TierCandidate EvidenceTier = "candidate"
	// TierUnresolved: applicability not established; no hardware asserted.
	TierUnresolved EvidenceTier = "unresolved"
)

// HardwareProfile describes a known ECU/module hardware variant (for example
// an engine ECU part number family). It is factory reference data, not a claim
// about what is installed in a specific vehicle.
type HardwareProfile struct {
	SchemaVersion string       `json:"schemaVersion"`
	ID            string       `json:"id"`
	DisplayName   string       `json:"displayName"`
	Kind          ModuleKind   `json:"kind"`
	Manufacturer  Fact[string] `json:"manufacturer"`
	PartNumber    Fact[string] `json:"partNumber"`
	Notes         string       `json:"notes,omitempty"`
}

// ConflictStatus is the resolution state of a structured conflict record.
type ConflictStatus string

const (
	ConflictOpen     ConflictStatus = "open"
	ConflictResolved ConflictStatus = "resolved"
)

// ConflictClaim is one side of a disputed attribute: the claimed value, its
// source, and optionally the catalog entry asserting it.
type ConflictClaim struct {
	Value  string `json:"value"`
	Source string `json:"source"`
	Entry  string `json:"entry,omitempty"`
}

// ConflictRecord documents a structured disagreement: which attribute is
// disputed, the competing claims with provenance, and whether it is resolved.
type ConflictRecord struct {
	Attribute string          `json:"attribute"`
	Claims    []ConflictClaim `json:"claims"`
	Status    ConflictStatus  `json:"status"`
	Notes     string          `json:"notes,omitempty"`
}

// CatalogEntry links one vehicle configuration to compatibility evidence for
// one module kind. Hardware is a fact reference to a HardwareProfile ID and
// must be empty for unresolved entries. Applicability holds structured
// fitment constraints evaluated by the resolver. Conflicts are structured
// records of contradictory evidence, never silently resolved.
type CatalogEntry struct {
	ID            string           `json:"id"`
	Configuration Fact[string]     `json:"configuration"`
	Module        ModuleKind       `json:"module"`
	Hardware      Fact[string]     `json:"hardware"`
	Tier          EvidenceTier     `json:"tier"`
	Applicability *Applicability   `json:"applicability,omitempty"`
	Evidence      []Evidence       `json:"evidence,omitempty"`
	Conflicts     []ConflictRecord `json:"conflicts,omitempty"`
	Notes         string           `json:"notes,omitempty"`
}

// UnmarshalJSON rejects an explicit null applicability, which the schema
// forbids but encoding/json would otherwise decode as an omitted field.
func (e *CatalogEntry) UnmarshalJSON(data []byte) error {
	type alias CatalogEntry
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, v := range raw {
		// encoding/json matches field names case-insensitively.
		if strings.EqualFold(key, "applicability") && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return errors.New("applicability must be an object when present")
		}
	}
	decoded := alias(*e)
	if err := decodeStrict(data, &decoded); err != nil {
		return err
	}
	*e = CatalogEntry(decoded)
	return nil
}

// CompatibilityCatalog is a versioned set of catalog entries scoped to one or
// more vehicle configurations.
type CompatibilityCatalog struct {
	SchemaVersion string         `json:"schemaVersion"`
	ID            string         `json:"id"`
	DisplayName   string         `json:"displayName"`
	Entries       []CatalogEntry `json:"entries"`
}

var moduleKinds = map[ModuleKind]bool{
	ModuleEngineECU:              true,
	ModuleABSController:          true,
	ModuleABSHydraulicAssembly:   true,
	ModuleTransmissionController: true,
	ModuleOther:                  true,
}

var evidenceTiers = map[EvidenceTier]bool{
	TierConfirmed:    true,
	TierCorroborated: true,
	TierCandidate:    true,
	TierUnresolved:   true,
}

func validTier(t EvidenceTier) bool { return evidenceTiers[t] }

func (h HardwareProfile) Validate() error {
	var problems []error
	if h.SchemaVersion != CatalogSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %q", CatalogSchemaVersion))
	}
	if strings.TrimSpace(h.ID) == "" {
		problems = append(problems, errors.New("id is required"))
	} else if !profileIDPattern.MatchString(h.ID) {
		problems = append(problems, fmt.Errorf("id must match %s", profileIDPattern.String()))
	}
	if strings.TrimSpace(h.DisplayName) == "" {
		problems = append(problems, errors.New("displayName is required"))
	}
	if !moduleKinds[h.Kind] {
		problems = append(problems, fmt.Errorf("kind: unsupported module kind %q", h.Kind))
	}
	problems = append(problems, checkStringFacts(map[string]Fact[string]{
		"manufacturer": h.Manufacturer, "partNumber": h.PartNumber,
	})...)
	return errors.Join(problems...)
}

func (e CatalogEntry) Validate() error {
	var problems []error
	if strings.TrimSpace(e.ID) == "" {
		problems = append(problems, errors.New("entry id is required"))
	}
	if err := validateStringFact("configuration", e.Configuration); err != nil {
		problems = append(problems, err)
	}
	if !moduleKinds[e.Module] {
		problems = append(problems, fmt.Errorf("module: unsupported module kind %q", e.Module))
	}
	if err := validateStringFact("hardware", e.Hardware); err != nil {
		problems = append(problems, err)
	}
	if !validTier(e.Tier) {
		problems = append(problems, fmt.Errorf("tier: invalid evidence tier %q", e.Tier))
	}

	hasHardware := e.Hardware.Value != nil
	switch e.Tier {
	case TierConfirmed, TierCorroborated, TierCandidate:
		if !hasHardware {
			problems = append(problems, fmt.Errorf("tier %q requires a hardware reference", e.Tier))
		}
		if len(e.Evidence) == 0 {
			problems = append(problems, fmt.Errorf("tier %q requires at least one evidence item", e.Tier))
		}
	case TierUnresolved:
		if hasHardware {
			problems = append(problems, errors.New("tier \"unresolved\" must not assert a hardware reference"))
		}
	}
	if e.Tier == TierCorroborated && distinctEvidenceSources(e.Evidence) < 2 {
		problems = append(problems, errors.New("tier \"corroborated\" requires evidence from at least two distinct sources"))
	}
	for i, item := range e.Evidence {
		if strings.TrimSpace(item.Source) == "" {
			problems = append(problems, fmt.Errorf("evidence[%d].source is required", i))
		}
	}
	for i, c := range e.Conflicts {
		if err := c.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("conflicts[%d]: %w", i, err))
		}
	}
	if e.Applicability != nil {
		if err := e.Applicability.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("applicability: %w", err))
		}
	}
	return errors.Join(problems...)
}

// Validate checks a structured conflict record: a known status, at least two
// competing claims, and non-empty claim values/sources.
func (c ConflictRecord) Validate() error {
	var problems []error
	switch c.Status {
	case ConflictOpen, ConflictResolved:
	default:
		problems = append(problems, fmt.Errorf("invalid conflict status %q", c.Status))
	}
	if strings.TrimSpace(c.Attribute) == "" {
		problems = append(problems, errors.New("attribute is required"))
	}
	if len(c.Claims) < 2 {
		problems = append(problems, errors.New("at least two competing claims are required"))
	}
	for i, claim := range c.Claims {
		if strings.TrimSpace(claim.Value) == "" {
			problems = append(problems, fmt.Errorf("claims[%d].value is required", i))
		}
		if strings.TrimSpace(claim.Source) == "" {
			problems = append(problems, fmt.Errorf("claims[%d].source is required", i))
		}
	}
	return errors.Join(problems...)
}

// distinctEvidenceSources counts unique non-empty evidence source labels.
func distinctEvidenceSources(evidence []Evidence) int {
	seen := map[string]bool{}
	for _, e := range evidence {
		if s := strings.TrimSpace(e.Source); s != "" {
			seen[s] = true
		}
	}
	return len(seen)
}

func (c CompatibilityCatalog) Validate() error {
	var problems []error
	if c.SchemaVersion != CompatibilityCatalogSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %q", CompatibilityCatalogSchemaVersion))
	}
	if strings.TrimSpace(c.ID) == "" {
		problems = append(problems, errors.New("id is required"))
	} else if !profileIDPattern.MatchString(c.ID) {
		problems = append(problems, fmt.Errorf("id must match %s", profileIDPattern.String()))
	}
	if strings.TrimSpace(c.DisplayName) == "" {
		problems = append(problems, errors.New("displayName is required"))
	}
	seen := map[string]bool{}
	for i, entry := range c.Entries {
		if err := entry.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("entries[%d]: %w", i, err))
		}
		if seen[entry.ID] {
			problems = append(problems, fmt.Errorf("entries[%d]: duplicate entry id %q", i, entry.ID))
		}
		seen[entry.ID] = true
	}
	return errors.Join(problems...)
}

// RegisterHardwareProfile validates and stores a hardware profile.
func (r *Registry) RegisterHardwareProfile(h HardwareProfile) error {
	return register(r.Hardware, h.ID, h, func(d HardwareProfile) error { return d.Validate() })
}

// RegisterCatalog validates and stores a compatibility catalog.
func (r *Registry) RegisterCatalog(c CompatibilityCatalog) error {
	return register(r.Catalogs, c.ID, c, func(d CompatibilityCatalog) error { return d.Validate() })
}

// ResolveCatalog checks that every valued configuration and hardware
// reference in the catalog resolves to a registered definition.
func (r *Registry) ResolveCatalog(c CompatibilityCatalog) error {
	var problems []error
	for i, e := range c.Entries {
		if e.Configuration.Value != nil {
			if _, ok := r.Configurations[*e.Configuration.Value]; !ok {
				problems = append(problems, fmt.Errorf("entries[%d]: no registered configuration %q", i, *e.Configuration.Value))
			}
		}
		if e.Hardware.Value != nil {
			hw, ok := r.Hardware[*e.Hardware.Value]
			if !ok {
				problems = append(problems, fmt.Errorf("entries[%d]: no registered hardware profile %q", i, *e.Hardware.Value))
			} else if hw.Kind != e.Module {
				problems = append(problems, fmt.Errorf("entries[%d]: hardware profile %q is kind %q, entry module is %q", i, hw.ID, hw.Kind, e.Module))
			}
		}
	}
	return errors.Join(problems...)
}

// Conflicts reports configuration/module pairs where entries name different
// hardware profiles. Any disagreement — even between candidates — is surfaced
// explicitly rather than silently resolved.
func (c CompatibilityCatalog) Conflicts() []string {
	type key struct {
		config string
		module ModuleKind
	}
	seen := map[key]map[string]string{}
	var out []string
	for _, e := range c.Entries {
		if e.Configuration.Value == nil || e.Hardware.Value == nil {
			continue
		}
		k := key{config: *e.Configuration.Value, module: e.Module}
		hw := *e.Hardware.Value
		if m, ok := seen[k]; ok {
			for prevID := range m {
				if prevID != hw {
					out = append(out, fmt.Sprintf("%s/%s: entries disagree on hardware (%s vs %s)", k.config, k.module, prevID, hw))
				}
			}
			m[hw] = e.ID
		} else {
			seen[k] = map[string]string{hw: e.ID}
		}
	}
	sort.Strings(out)
	return out
}

// CatalogCandidates returns entries for a configuration and module ordered by
// evidence strength (confirmed, corroborated, candidate, unresolved). It only
// reports compatibility evidence; it never asserts what hardware is installed.
func (c CompatibilityCatalog) CatalogCandidates(configurationID string, module ModuleKind) []CatalogEntry {
	var out []CatalogEntry
	for _, e := range c.Entries {
		if e.Module == module && e.Configuration.Value != nil && *e.Configuration.Value == configurationID {
			out = append(out, e)
		}
	}
	rank := map[EvidenceTier]int{TierConfirmed: 0, TierCorroborated: 1, TierCandidate: 2, TierUnresolved: 3}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Tier] < rank[out[j].Tier] })
	return out
}
