package vehicle

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// CatalogSchemaVersion versions the hardware profile and compatibility
// catalog document formats.
const CatalogSchemaVersion = "1.0.0"

// ModuleKind identifies the vehicle module a catalog entry applies to.
type ModuleKind string

const (
	ModuleEngineECU              ModuleKind = "engine-ecu"
	ModuleABSController          ModuleKind = "abs-controller"
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

// CatalogEntry links one vehicle configuration to compatibility evidence for
// one module kind. Hardware is a fact reference to a HardwareProfile ID and
// must be empty for unresolved entries. Applicability is an optional production
// range fact. Conflicts record contradictory evidence explicitly instead of
// silently resolving it.
type CatalogEntry struct {
	ID            string                 `json:"id"`
	Configuration Fact[string]           `json:"configuration"`
	Module        ModuleKind             `json:"module"`
	Hardware      Fact[string]           `json:"hardware"`
	Tier          EvidenceTier           `json:"tier"`
	Applicability *Fact[ProductionRange] `json:"applicability,omitempty"`
	Evidence      []Evidence             `json:"evidence,omitempty"`
	Conflicts     []string               `json:"conflicts,omitempty"`
	Notes         string                 `json:"notes,omitempty"`
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
	if err := validateFact("hardware", e.Hardware); err != nil {
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
	for i, item := range e.Evidence {
		if strings.TrimSpace(item.Source) == "" {
			problems = append(problems, fmt.Errorf("evidence[%d].source is required", i))
		}
	}
	if e.Applicability != nil {
		if err := validateFact("applicability", *e.Applicability); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func (c CompatibilityCatalog) Validate() error {
	var problems []error
	if c.SchemaVersion != CatalogSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %q", CatalogSchemaVersion))
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
			if _, ok := r.Hardware[*e.Hardware.Value]; !ok {
				problems = append(problems, fmt.Errorf("entries[%d]: no registered hardware profile %q", i, *e.Hardware.Value))
			}
		}
	}
	return errors.Join(problems...)
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
