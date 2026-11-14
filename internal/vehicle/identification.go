package vehicle

import "fmt"

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
// specific vehicle. The full VIN is never stored — only the masked form.
type VehicleIdentification struct {
	VIN       Fact[string] `json:"vin"`
	VINDecode *VINDecode   `json:"vinDecode,omitempty"`
	ModelYear Fact[int]    `json:"modelYear"`
	Make      Fact[string] `json:"make"`
	Model     Fact[string] `json:"model"`
	Trim      Fact[string] `json:"trim"`
}

// ManualStringFact builds a confirmed fact sourced from manual entry.
func ManualStringFact(value string) Fact[string] {
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

// ApplyVIN decodes raw and records the masked VIN plus whatever the VIN
// deterministically provides. The full VIN is discarded after masking.
//
// Status rules:
//   - complete VIN with valid check digit: confirmed
//   - complete VIN with bad check digit or partial VIN: inferred, with notes
//
// ModelYear is only populated when contextYear resolves the 30-year code
// ambiguity to a single candidate; otherwise it stays unknown and the
// candidates remain available on VINDecode.
func (id *VehicleIdentification) ApplyVIN(raw, contextYearHint string) error {
	d := DecodeVIN(raw)
	if d.VIN == "" {
		return fmt.Errorf("VIN is empty")
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

	if y, ok := resolveContextYear(d, contextYearHint); ok {
		id.ModelYear = Fact[int]{
			Value:    &y,
			Status:   StatusInferred,
			Evidence: []Evidence{{Source: string(SourceVINDecode), Description: "model year decoded from VIN position 10 within vehicle context"}},
			Notes:    "inferred from VIN year code and vehicle context; confirm against build documentation",
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
