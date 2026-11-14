package vehicle

import (
	"errors"
	"fmt"
	"strings"
)

// IdentificationSource records how a vehicle identification field was
// established, so decoded attributes are never confused with user entries.
type IdentificationSource string

const (
	// SourceVINDecode marks fields populated by DecodeVIN.
	SourceVINDecode IdentificationSource = "vin-decode"
	// SourceManualEntry marks fields entered or confirmed by the user.
	SourceManualEntry IdentificationSource = "manual-entry"
)

// VehicleIdentification is the working identification record for a vehicle.
// It is distinct from the factory VehicleProfile: profile facts come from
// curated definitions, while identification captures what was observed for a
// specific vehicle. The full VIN is retained only transiently in VINDecode;
// JSON serialization omits its raw and normalized forms.
type VehicleIdentification struct {
	VIN       Fact[string] `json:"vin"`
	VINDecode *VINDecode   `json:"vinDecode,omitempty"`
	ModelYear Fact[int]    `json:"modelYear"`
	Make      Fact[string] `json:"make"`
	Model     Fact[string] `json:"model"`
	Trim      Fact[string] `json:"trim"`
}

// ManualStringFact builds a confirmed fact sourced from manual entry.
// Blank input remains explicitly unknown rather than becoming a confirmed fact.
func ManualStringFact(value string) Fact[string] {
	value = strings.TrimSpace(value)
	if value == "" {
		return Fact[string]{Status: StatusUnknown}
	}
	return Fact[string]{
		Value:    &value,
		Status:   StatusConfirmed,
		Evidence: []Evidence{{Source: string(SourceManualEntry)}},
	}
}

// ManualIntFact builds a confirmed integer fact sourced from manual entry.
func ManualIntFact(value int) Fact[int] {
	return Fact[int]{
		Value:    &value,
		Status:   StatusConfirmed,
		Evidence: []Evidence{{Source: string(SourceManualEntry)}},
	}
}

// NewVehicleIdentification returns an identification with every field
// explicitly unknown — nothing is invented.
func NewVehicleIdentification() *VehicleIdentification {
	return &VehicleIdentification{
		VIN:       Fact[string]{Status: StatusUnknown},
		ModelYear: Fact[int]{Status: StatusUnknown},
		Make:      Fact[string]{Status: StatusUnknown},
		Model:     Fact[string]{Status: StatusUnknown},
		Trim:      Fact[string]{Status: StatusUnknown},
	}
}

// Validate checks every identification fact, including fields assigned
// directly by callers instead of using the manual-entry helpers.
func (id VehicleIdentification) Validate() error {
	var problems []error
	if err := validateStringFact("vin", id.VIN); err != nil {
		problems = append(problems, err)
	}
	if err := validateFact("modelYear", id.ModelYear); err != nil {
		problems = append(problems, err)
	}
	if id.ModelYear.Value != nil && (*id.ModelYear.Value < 1900 || *id.ModelYear.Value > 2100) {
		problems = append(problems, errors.New("modelYear: value must be between 1900 and 2100"))
	}
	for name, fact := range map[string]Fact[string]{
		"make": id.Make, "model": id.Model, "trim": id.Trim,
	} {
		if err := validateStringFact(name, fact); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

// ApplyVIN decodes raw and records the masked VIN plus whatever the VIN
// deterministically provides. The full VIN is discarded from the persisted
// representation after masking.
//
// Status rules:
//   - complete VIN with valid check digit: confirmed
//   - complete VIN with bad check digit or partial VIN: inferred, with notes
//
// ModelYear is only populated when contextYear resolves the 30-year code
// ambiguity to a single candidate; otherwise it stays unknown unless it was
// explicitly confirmed by manual entry.
func (id *VehicleIdentification) ApplyVIN(raw, contextYearHint string) error {
	d := DecodeVIN(raw)
	if d.VIN == "" {
		return fmt.Errorf("VIN is empty")
	}

	previousYearSource := FieldSource(id.ModelYear)
	if previousYearSource == string(SourceVINDecode) {
		// A previous VIN-derived year belongs to the old VIN and must not leak
		// into the newly applied VIN when its year code cannot be resolved.
		id.ModelYear = Fact[int]{Status: StatusUnknown}
	}

	masked := d.Masked()
	status := StatusConfirmed
	notes := ""
	switch {
	case len(d.VIN) < VINLength:
		status = StatusInferred
		notes = "partial VIN; check digit not applicable"
	case !d.Complete:
		status = StatusInferred
		notes = "VIN contains invalid characters"
	case d.CheckDigit == CheckDigitInvalid:
		status = StatusInferred
		notes = "check digit mismatch; VIN may be misread"
	}
	id.VIN = Fact[string]{
		Value:    &masked,
		Status:   status,
		Evidence: []Evidence{{Source: string(SourceVINDecode), Description: "masked VIN; full VIN not persisted"}},
		Notes:    notes,
	}
	decode := d
	id.VINDecode = &decode

	if previousYearSource != string(SourceManualEntry) {
		if y, ok := resolveContextYear(d, contextYearHint); ok {
			id.ModelYear = Fact[int]{
				Value:    &y,
				Status:   StatusInferred,
				Evidence: []Evidence{{Source: string(SourceVINDecode), Description: "model year decoded from VIN position 10 within vehicle context"}},
				Notes:    "inferred from VIN year code and vehicle context; confirm against build documentation",
			}
		}
	}
	return nil
}

// FieldSource reports the evidence source for a fact, or "" when the fact has
// no value. Used to distinguish decoded vs user-entered attributes.
func FieldSource[T any](f Fact[T]) string {
	if f.Value == nil || len(f.Evidence) == 0 {
		return ""
	}
	return f.Evidence[0].Source
}

func resolveContextYear(d VINDecode, hint string) (int, bool) {
	if len(d.YearCandidates) == 1 {
		return d.YearCandidates[0], true
	}
	var year int
	if _, err := fmt.Sscanf(hint, "%d", &year); err != nil {
		return 0, false
	}
	return d.ResolveYear(year)
}
