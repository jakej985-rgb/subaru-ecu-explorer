package vehicle

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// VINLength is the standard 17-character VIN length.
const VINLength = 17

// vinAlphabet excludes I, O, and Q per ISO 3779 / 49 CFR 565.
const vinAlphabet = "0123456789ABCDEFGHJKLMNPRSTUVWXYZ"

// vinValues maps VIN characters to their check-digit transliteration values.
var vinValues = map[byte]int{
	'0': 0, '1': 1, '2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8, '9': 9,
	'A': 1, 'B': 2, 'C': 3, 'D': 4, 'E': 5, 'F': 6, 'G': 7, 'H': 8,
	'J': 1, 'K': 2, 'L': 3, 'M': 4, 'N': 5,
	'P': 7, 'R': 9,
	'S': 2, 'T': 3, 'U': 4, 'V': 5, 'W': 6, 'X': 7, 'Y': 8, 'Z': 9,
}

// vinWeights are the check-digit weights for VIN positions 1-17.
var vinWeights = [VINLength]int{8, 7, 6, 5, 4, 3, 2, 10, 0, 9, 8, 7, 6, 5, 4, 3, 2}

// yearCycleChars lists the model-year codes for one 30-year cycle starting in
// 1980: A(1980) ... Y(2000), 1-9 (2001-2009). Codes repeat every 30 years.
const yearCycleChars = "ABCDEFGHJKLMNPRSTVWXY123456789"

const (
	vinYearCycleStart = 1980
	vinYearCycleLen   = 30
	// Plausible decode window for North American VINs.
	vinYearMin = 1980
	vinYearMax = 2039
)

// CheckDigitStatus describes the outcome of the ISO 3779 check-digit test.
type CheckDigitStatus string

const (
	CheckDigitValid         CheckDigitStatus = "valid"
	CheckDigitInvalid       CheckDigitStatus = "invalid"
	CheckDigitNotApplicable CheckDigitStatus = "not-applicable"
)

// VINDecode is a tolerant decode result. It never fails outright: malformed
// input is recorded in Errors and Complete stays false. Only structurally
// valid positions are decoded; nothing is guessed.
type VINDecode struct {
	Input          string           `json:"-"` // raw input is transient and must never be serialized
	VIN            string           `json:"-"` // normalized full VIN is transient and must never be serialized
	Complete       bool             `json:"complete"`
	CheckDigit     CheckDigitStatus `json:"checkDigit"`
	WMI            string           `json:"wmi,omitempty"`
	VDS            string           `json:"vds,omitempty"`
	VIS            string           `json:"vis,omitempty"`
	YearCode       string           `json:"yearCode,omitempty"`
	YearCandidates []int            `json:"yearCandidates,omitempty"`
	PlantCode      string           `json:"plantCode,omitempty"`
	Errors         []string         `json:"errors,omitempty"`
}

// DecodeVIN normalizes and decodes a VIN candidate. Whitespace and dashes are
// stripped; characters are uppercased. Short or malformed input still returns
// a decode with Complete=false and populated Errors.
func DecodeVIN(input string) VINDecode {
	d := VINDecode{Input: input, CheckDigit: CheckDigitNotApplicable}
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(input)) {
		if r == '-' || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	d.VIN = b.String()

	for i := 0; i < len(d.VIN); i++ {
		if !strings.ContainsRune(vinAlphabet, rune(d.VIN[i])) {
			d.Errors = append(d.Errors, fmt.Sprintf("invalid VIN character %q at position %d", d.VIN[i], i+1))
		}
	}

	if len(d.VIN) < VINLength {
		d.Errors = append(d.Errors, fmt.Sprintf("VIN has %d characters; %d required", len(d.VIN), VINLength))
	} else if len(d.VIN) > VINLength {
		d.Errors = append(d.Errors, fmt.Sprintf("VIN has %d characters; %d required", len(d.VIN), VINLength))
		d.VIN = d.VIN[:VINLength]
	}

	n := len(d.VIN)
	if n >= 3 {
		d.WMI = d.VIN[:3]
	}
	if n >= 8 {
		d.VDS = d.VIN[3:8]
	}
	if n >= 17 {
		d.VIS = d.VIN[9:]
		d.Complete = len(d.Errors) == 0
	}
	if n >= 10 {
		d.YearCode = string(d.VIN[9])
		d.YearCandidates = yearCandidates(d.VIN[9])
	}
	if n >= 11 {
		d.PlantCode = string(d.VIN[10])
	}
	if n == VINLength && d.Complete {
		d.CheckDigit = checkDigitStatus(d.VIN)
		if d.CheckDigit == CheckDigitInvalid {
			d.Errors = append(d.Errors, "check digit does not match")
		}
	}
	return d
}

// Validate returns an error unless the decode is a complete, alphabet-valid,
// check-digit-valid 17-character VIN.
func (d VINDecode) Validate() error {
	if !d.Complete {
		return fmt.Errorf("incomplete VIN: %s", strings.Join(d.Errors, "; "))
	}
	if d.CheckDigit != CheckDigitValid {
		return fmt.Errorf("VIN check digit %s", d.CheckDigit)
	}
	return nil
}

// ResolveYear picks the model year from YearCandidates matching the given
// vehicle context year. It reports false when the context is outside the
// candidate set, so callers must not silently pick a year.
func (d VINDecode) ResolveYear(contextYear int) (int, bool) {
	for _, y := range d.YearCandidates {
		if y == contextYear {
			return y, true
		}
	}
	return 0, false
}

// Masked returns a privacy-preserving identifier: the WMI (manufacturer
// region, not vehicle-unique) plus the last 6 characters, with the middle
// masked. Partial VINs are fully masked. The full VIN must never be persisted.
func (d VINDecode) Masked() string {
	v := d.VIN
	if len(v) < VINLength {
		return strings.Repeat("*", len(v))
	}
	return v[:3] + strings.Repeat("*", 8) + v[11:]
}

func checkDigitStatus(vin string) CheckDigitStatus {
	sum := 0
	for i := 0; i < VINLength; i++ {
		sum += vinValues[vin[i]] * vinWeights[i]
	}
	rem := sum % 11
	computed := byte('0' + rem)
	if rem == 10 {
		computed = 'X'
	}
	if vin[8] == computed {
		return CheckDigitValid
	}
	return CheckDigitInvalid
}

func yearCandidates(code byte) []int {
	idx := strings.IndexByte(yearCycleChars, code)
	if idx < 0 {
		return nil
	}
	var out []int
	for y := vinYearCycleStart + idx; y <= vinYearMax; y += vinYearCycleLen {
		out = append(out, y)
	}
	sort.Ints(out)
	return out
}
