package vehicle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "vin", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return strings.TrimSpace(string(data))
}

func TestDecodeVINValidFixture(t *testing.T) {
	d := DecodeVIN(fixture(t, "valid.txt"))
	if err := d.Validate(); err != nil {
		t.Fatalf("expected valid VIN, got %v (errors: %v)", err, d.Errors)
	}
	if d.VIN != "JF1CE4G22XH000001" {
		t.Fatalf("unexpected normalized VIN %q", d.VIN)
	}
	if d.WMI != "JF1" || d.VDS != "CE4G22" || d.VIS != "XH000001" {
		t.Fatalf("unexpected segments wmi=%q vds=%q vis=%q", d.WMI, d.VDS, d.VIS)
	}
	if d.YearCode != "X" {
		t.Fatalf("expected year code X, got %q", d.YearCode)
	}
	if len(d.YearCandidates) != 2 || d.YearCandidates[0] != 1999 || d.YearCandidates[1] != 2029 {
		t.Fatalf("expected year candidates [1999 2029], got %v", d.YearCandidates)
	}
	if d.PlantCode != "H" {
		t.Fatalf("expected plant code H, got %q", d.PlantCode)
	}
}

func TestDecodeVINFormattedInput(t *testing.T) {
	d := DecodeVIN(fixture(t, "formatted.txt"))
	if err := d.Validate(); err != nil {
		t.Fatalf("formatted VIN should decode, got %v", err)
	}
	if d.VIN != "JF1CE4G22XH000001" {
		t.Fatalf("expected normalization to strip separators, got %q", d.VIN)
	}
}

func TestDecodeVINInvalidCheckDigit(t *testing.T) {
	d := DecodeVIN(fixture(t, "invalid-checkdigit.txt"))
	if d.CheckDigit != CheckDigitInvalid {
		t.Fatalf("expected invalid check digit, got %s", d.CheckDigit)
	}
	if !d.Complete {
		t.Fatal("expected Complete=true: structure is well-formed, only check digit fails")
	}
	if err := d.Validate(); err == nil {
		t.Fatal("expected Validate to reject bad check digit")
	}
}

func TestDecodeVINInvalidCharacters(t *testing.T) {
	d := DecodeVIN(fixture(t, "invalid-chars.txt"))
	found := false
	for _, e := range d.Errors {
		if strings.Contains(e, "invalid VIN character") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected invalid character error, got %v", d.Errors)
	}
	if d.Complete {
		t.Fatal("expected Complete=false for invalid characters")
	}
}

func TestDecodeVINPartial(t *testing.T) {
	d := DecodeVIN(fixture(t, "partial.txt"))
	if d.Complete {
		t.Fatal("expected partial VIN to be incomplete")
	}
	if d.CheckDigit != CheckDigitNotApplicable {
		t.Fatalf("expected check digit not-applicable, got %s", d.CheckDigit)
	}
	if len(d.Errors) == 0 {
		t.Fatal("expected length error for partial VIN")
	}
	if d.YearCode != "X" || len(d.YearCandidates) != 2 {
		t.Fatalf("partial VIN should still decode year code, got %q %v", d.YearCode, d.YearCandidates)
	}
}

func TestVINMasked(t *testing.T) {
	d := DecodeVIN(fixture(t, "valid.txt"))
	masked := d.Masked()
	if masked != "JF1********000001" {
		t.Fatalf("unexpected masked form %q", masked)
	}
	if strings.Contains(masked, "CE4G22") {
		t.Fatal("masked VIN must not contain the VDS")
	}
	partial := DecodeVIN(fixture(t, "partial.txt"))
	if partial.Masked() != strings.Repeat("*", len(partial.VIN)) {
		t.Fatalf("partial VIN should be fully masked, got %q", partial.Masked())
	}
}

func TestVINResolveYear(t *testing.T) {
	d := DecodeVIN(fixture(t, "valid.txt"))
	if y, ok := d.ResolveYear(1999); !ok || y != 1999 {
		t.Fatalf("expected 1999 to resolve, got %d %v", y, ok)
	}
	if _, ok := d.ResolveYear(2005); ok {
		t.Fatal("expected 2005 not to resolve for year code X")
	}
}

func TestApplyVINConfirmed(t *testing.T) {
	id := NewVehicleIdentification()
	if err := id.ApplyVIN(fixture(t, "valid.txt"), "1999"); err != nil {
		t.Fatalf("ApplyVIN: %v", err)
	}
	if id.VIN.Status != StatusConfirmed {
		t.Fatalf("expected confirmed VIN fact, got %s", id.VIN.Status)
	}
	if id.VIN.Value == nil || *id.VIN.Value != "JF1********000001" {
		t.Fatalf("expected masked VIN value, got %+v", id.VIN.Value)
	}
	if FieldSource(id.VIN) != string(SourceVINDecode) {
		t.Fatalf("expected vin-decode source, got %q", FieldSource(id.VIN))
	}
	if id.ModelYear.Status != StatusInferred || id.ModelYear.Value == nil || *id.ModelYear.Value != 1999 {
		t.Fatalf("expected inferred model year 1999, got %+v", id.ModelYear)
	}
	if FieldSource(id.ModelYear) != string(SourceVINDecode) {
		t.Fatalf("expected vin-decode source for year, got %q", FieldSource(id.ModelYear))
	}
}

func TestApplyVINWithoutContextLeavesYearUnknown(t *testing.T) {
	id := NewVehicleIdentification()
	if err := id.ApplyVIN(fixture(t, "valid.txt"), ""); err != nil {
		t.Fatalf("ApplyVIN: %v", err)
	}
	if id.ModelYear.Status != StatusUnknown || id.ModelYear.Value != nil {
		t.Fatalf("ambiguous year code must stay unknown, got %+v", id.ModelYear)
	}
	if id.VINDecode == nil || len(id.VINDecode.YearCandidates) != 2 {
		t.Fatal("year candidates should be preserved on the decode")
	}
}

func TestApplyVINPartialIsInferred(t *testing.T) {
	id := NewVehicleIdentification()
	if err := id.ApplyVIN(fixture(t, "partial.txt"), "1999"); err != nil {
		t.Fatalf("ApplyVIN partial: %v", err)
	}
	if id.VIN.Status != StatusInferred {
		t.Fatalf("expected inferred partial VIN, got %s", id.VIN.Status)
	}
	if *id.VIN.Value != strings.Repeat("*", len("JF1CE4G22XH")) {
		t.Fatalf("partial VIN must be fully masked, got %q", *id.VIN.Value)
	}
}

func TestApplyVINBadCheckDigitIsInferred(t *testing.T) {
	id := NewVehicleIdentification()
	if err := id.ApplyVIN(fixture(t, "invalid-checkdigit.txt"), "1999"); err != nil {
		t.Fatalf("ApplyVIN: %v", err)
	}
	if id.VIN.Status != StatusInferred {
		t.Fatalf("expected inferred status for bad check digit, got %s", id.VIN.Status)
	}
}

func TestManualEntryFallback(t *testing.T) {
	id := NewVehicleIdentification()
	if err := id.ApplyVIN(fixture(t, "partial.txt"), ""); err != nil {
		t.Fatalf("ApplyVIN partial: %v", err)
	}

	// Manual fallback: user-entered fields are confirmed with manual-entry source.
	id.Make = ManualStringFact("Subaru")
	id.Model = ManualStringFact("Legacy Outback")
	year := 1999
	id.ModelYear = Fact[int]{Value: &year, Status: StatusConfirmed, Evidence: []Evidence{{Source: string(SourceManualEntry)}}}

	if FieldSource(id.Make) != string(SourceManualEntry) {
		t.Fatalf("expected manual-entry source, got %q", FieldSource(id.Make))
	}
	if FieldSource(id.ModelYear) != string(SourceManualEntry) {
		t.Fatalf("expected manual-entry source for year, got %q", FieldSource(id.ModelYear))
	}
	if FieldSource(id.VIN) != string(SourceVINDecode) {
		t.Fatalf("VIN should still be vin-decode sourced, got %q", FieldSource(id.VIN))
	}
}

func TestEmptyVINRejected(t *testing.T) {
	id := NewVehicleIdentification()
	if err := id.ApplyVIN("  --  ", "1999"); err == nil {
		t.Fatal("expected error for empty VIN")
	}
}
