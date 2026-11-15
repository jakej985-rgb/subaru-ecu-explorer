package vehicle

import (
	"fmt"
	"sort"
	"strings"
)

// ConstraintState is the three-valued result of comparing one applicability
// constraint against a vehicle fact. Missing vehicle facts never count as a
// match.
type ConstraintState string

const (
	// ConstraintMatch: constraint supplied and vehicle fact agrees.
	ConstraintMatch ConstraintState = "match"
	// ConstraintConflict: constraint supplied and vehicle fact disagrees.
	ConstraintConflict ConstraintState = "conflict"
	// ConstraintUnknown: constraint supplied but the vehicle fact is missing
	// or ambiguous (partial VIN, unresolved configuration reference).
	ConstraintUnknown ConstraintState = "unknown"
	// ConstraintNotSupplied: the entry places no constraint on this dimension.
	ConstraintNotSupplied ConstraintState = "not-supplied"
)

// ConstraintResult explains one dimension of an applicability evaluation.
type ConstraintResult struct {
	Dimension string          `json:"dimension"`
	State     ConstraintState `json:"state"`
	Expected  string          `json:"expected,omitempty"`
	Actual    string          `json:"actual,omitempty"`
	Detail    string          `json:"detail,omitempty"`
}

// FitOutcome is the resolver's verdict for one catalog entry. Evidence tier
// and fitment outcome are independent dimensions: a confirmed-tier entry can
// still be incompatible, and a candidate-tier entry can fit cleanly.
type FitOutcome string

const (
	// FitCompatibleCandidate: all supplied constraints match; entry is a
	// plausible fit. Still not proof of installed hardware.
	FitCompatibleCandidate FitOutcome = "compatible-candidate"
	// FitIncompatible: at least one supplied constraint conflicts with a
	// known vehicle fact. Tier cannot override this.
	FitIncompatible FitOutcome = "incompatible"
	// FitInsufficientInfo: no conflicts, but at least one supplied constraint
	// could not be evaluated because a vehicle fact is unknown.
	FitInsufficientInfo FitOutcome = "insufficient-information"
	// FitConflictingEvidence: the entry carries open conflict records;
	// its own claims are disputed.
	FitConflictingEvidence FitOutcome = "conflicting-evidence"
	// FitUnresolved: the entry's tier is unresolved — it asserts no hardware
	// and no applicability, so there is nothing to fit.
	FitUnresolved FitOutcome = "unresolved"
)

// EntryResolution is the explainable result for one catalog entry.
type EntryResolution struct {
	EntryID     string             `json:"entryId"`
	Module      ModuleKind         `json:"module"`
	Tier        EvidenceTier       `json:"tier"`
	Outcome     FitOutcome         `json:"outcome"`
	Hardware    string             `json:"hardware,omitempty"`
	Constraints []ConstraintResult `json:"constraints"`
	Evidence    []Evidence         `json:"evidence,omitempty"`
	Conflicts   []ConflictRecord   `json:"conflicts,omitempty"`
	Reasons     []string           `json:"reasons"`
}

// FitmentReport is the full resolver output for one configuration.
type FitmentReport struct {
	ConfigurationID string
	Vehicle         *VehicleFacts
	Results         []EntryResolution // every evaluated entry, sorted by entry ID
	Compatible      []EntryResolution // compatible candidates ranked by tier then ID
}

// VehicleFacts is the resolved attribute view of a vehicle used for
// applicability checks. Facts that are unknown stay unknown; ambiguity from
// partial VINs is preserved in YearCandidates.
type VehicleFacts struct {
	ConfigurationID string
	ModelYear       Fact[int]
	YearCandidates  []int
	Market          Fact[string]
	Model           Fact[string]
	Body            Fact[string]
	Trim            Fact[string]
	Engine          Fact[string] // engine configuration ID reference
	Transmission    Fact[string]
	Drivetrain      Fact[string]
	Production      Fact[ProductionRange]
	// Options is always empty: factory option data is not yet modeled, so any
	// option constraint evaluates as unknown rather than assumed.
	Options map[string]string
}

var unknownStringFact = Fact[string]{Status: StatusUnknown}
var unknownIntFact = Fact[int]{Status: StatusUnknown}
var unknownProdFact = Fact[ProductionRange]{Status: StatusUnknown}

// VehicleFactsFor resolves the vehicle-side attributes for a configuration
// using the registry plus optional identification data (for example a partial
// VIN decode). A missing identification leaves VIN-derived facts unknown.
func (r *Registry) VehicleFactsFor(configurationID string, id *VehicleIdentification) (*VehicleFacts, error) {
	cfg, ok := r.Configurations[configurationID]
	if !ok {
		return nil, fmt.Errorf("no registered configuration %q", configurationID)
	}
	v := &VehicleFacts{
		ConfigurationID: configurationID,
		ModelYear:       unknownIntFact,
		Market:          unknownStringFact,
		Model:           unknownStringFact,
		Body:            unknownStringFact,
		Trim:            unknownStringFact,
		Engine:          cfg.Engine,
		Transmission:    cfg.Transmission,
		Drivetrain:      cfg.Drivetrain,
		Production:      unknownProdFact,
		Options:         map[string]string{},
	}

	if cfg.Profile.Value != nil {
		if p, ok := r.Profiles[*cfg.Profile.Value]; ok {
			v.ModelYear = p.ModelYear
			v.Market = p.Market
			v.Model = p.Model
			v.Body = p.Body
			v.Trim = p.Trim
			if p.Production != nil {
				v.Production = *p.Production
			}
		}
	}

	if id != nil {
		v.applyIdentification(id)
	}
	return v, nil
}

// applyIdentification fills vehicle facts from a VIN decode / manual entry
// without overriding facts already established by the profile.
func (v *VehicleFacts) applyIdentification(id *VehicleIdentification) {
	// Model year: prefer an established fact; otherwise resolve VIN candidates.
	if v.ModelYear.Value == nil {
		if id.ModelYear.Value != nil {
			v.ModelYear = id.ModelYear
		} else if id.VINDecode != nil {
			v.YearCandidates = id.VINDecode.YearCandidates
			switch len(v.YearCandidates) {
			case 1:
				y := v.YearCandidates[0]
				v.ModelYear = Fact[int]{
					Value:    &y,
					Status:   StatusInferred,
					Evidence: []Evidence{{Source: string(SourceVINDecode), Description: "single model-year candidate from VIN"}},
				}
			default:
				// Ambiguous or absent: stays unknown.
			}
		}
	}
	// Manual-entry fields fill gaps the profile leaves unknown.
	fill := func(dst *Fact[string], src Fact[string]) {
		if dst.Value == nil && src.Value != nil {
			*dst = src
		}
	}
	fill(&v.Model, id.Model)
	fill(&v.Trim, id.Trim)
}

// ResolveFitment evaluates every catalog entry that targets configurationID
// against the resolved vehicle facts. Results are deterministic: Results is
// sorted by entry ID; Compatible preserves all passing entries ranked by
// evidence tier then entry ID — multiple plausible matches remain visible.
func (r *Registry) ResolveFitment(cat CompatibilityCatalog, configurationID string, id *VehicleIdentification) (*FitmentReport, error) {
	v, err := r.VehicleFactsFor(configurationID, id)
	if err != nil {
		return nil, err
	}
	report := &FitmentReport{ConfigurationID: configurationID, Vehicle: v}
	for _, e := range cat.Entries {
		if e.Configuration.Value == nil || *e.Configuration.Value != configurationID {
			continue
		}
		report.Results = append(report.Results, r.resolveEntry(e, v))
	}
	sort.Slice(report.Results, func(i, j int) bool { return report.Results[i].EntryID < report.Results[j].EntryID })
	for _, res := range report.Results {
		if res.Outcome == FitCompatibleCandidate {
			report.Compatible = append(report.Compatible, res)
		}
	}
	tierRank := map[EvidenceTier]int{TierConfirmed: 0, TierCorroborated: 1, TierCandidate: 2, TierUnresolved: 3}
	sort.SliceStable(report.Compatible, func(i, j int) bool {
		ri, rj := tierRank[report.Compatible[i].Tier], tierRank[report.Compatible[j].Tier]
		if ri != rj {
			return ri < rj
		}
		return report.Compatible[i].EntryID < report.Compatible[j].EntryID
	})
	return report, nil
}

func (r *Registry) resolveEntry(e CatalogEntry, v *VehicleFacts) EntryResolution {
	res := EntryResolution{
		EntryID:   e.ID,
		Module:    e.Module,
		Tier:      e.Tier,
		Evidence:  e.Evidence,
		Conflicts: e.Conflicts,
	}
	if e.Hardware.Value != nil {
		res.Hardware = *e.Hardware.Value
	}

	// Structured open conflicts dispute the entry's own claims.
	openConflicts := false
	for _, c := range e.Conflicts {
		if c.Status == ConflictOpen {
			openConflicts = true
		}
	}

	res.Constraints = evaluateApplicability(e.Applicability, v)

	// Unresolved entries assert no hardware; they cannot be fit candidates
	// regardless of how constraints evaluate.
	if e.Tier == TierUnresolved {
		res.Outcome = FitUnresolved
		res.Reasons = append(res.Reasons, "entry tier is unresolved; no hardware applicability asserted")
		return res
	}

	hasConflict, hasUnknown := false, false
	for _, c := range res.Constraints {
		switch c.State {
		case ConstraintConflict:
			hasConflict = true
			res.Reasons = append(res.Reasons, fmt.Sprintf("%s conflicts: expected %s, vehicle %s", c.Dimension, c.Expected, c.Actual))
		case ConstraintUnknown:
			hasUnknown = true
			res.Reasons = append(res.Reasons, fmt.Sprintf("%s unknown: %s", c.Dimension, c.Detail))
		}
	}

	switch {
	case openConflicts:
		res.Outcome = FitConflictingEvidence
		res.Reasons = append([]string{"entry has open conflict records"}, res.Reasons...)
	case hasConflict:
		res.Outcome = FitIncompatible
	case hasUnknown:
		res.Outcome = FitInsufficientInfo
	default:
		res.Outcome = FitCompatibleCandidate
		res.Reasons = append(res.Reasons, "all supplied applicability constraints match")
	}
	return res
}

// evaluateApplicability checks each supplied constraint dimension against the
// vehicle facts and returns one result per dimension in fixed order.
func evaluateApplicability(a *Applicability, v *VehicleFacts) []ConstraintResult {
	if a == nil {
		return nil
	}
	var out []ConstraintResult

	out = append(out, evalIntRange("modelYear", a.ModelYear, v.ModelYear, v.YearCandidates))
	out = append(out, evalString("market", a.Market, v.Market))
	out = append(out, evalString("model", a.Model, v.Model))
	out = append(out, evalString("body", a.Body, v.Body))
	out = append(out, evalString("engine", a.Engine, v.Engine))
	out = append(out, evalString("transmission", a.Transmission, v.Transmission))
	out = append(out, evalString("drivetrain", a.Drivetrain, v.Drivetrain))
	out = append(out, evalProduction(a.Production, v.Production))

	// Factory options are not modeled yet: every supplied option constraint is
	// unknown rather than assumed.
	optionNames := make([]string, 0, len(a.Options))
	for name := range a.Options {
		optionNames = append(optionNames, name)
	}
	sort.Strings(optionNames)
	for _, name := range optionNames {
		out = append(out, ConstraintResult{
			Dimension: "options." + name,
			State:     ConstraintUnknown,
			Expected:  optionExpected(a.Options[name]),
			Detail:    "vehicle factory options are not modeled; cannot evaluate",
		})
	}
	return out
}

func optionExpected(f Fact[string]) string {
	if f.Value != nil {
		return *f.Value
	}
	return string(f.Status)
}

// valuelessConstraint handles a present constraint whose value is not
// established (unknown/unsupported). It is never treated as not-supplied.
func valuelessConstraint(res ConstraintResult, status FactStatus) ConstraintResult {
	res.State = ConstraintUnknown
	res.Expected = string(status)
	res.Detail = fmt.Sprintf("constraint value is %s; cannot evaluate", status)
	return res
}

func evalString(dimension string, constraint *Fact[string], vehicle Fact[string]) ConstraintResult {
	res := ConstraintResult{Dimension: dimension}
	if constraint == nil {
		res.State = ConstraintNotSupplied
		return res
	}
	if constraint.Value == nil {
		return valuelessConstraint(res, constraint.Status)
	}
	res.Expected = *constraint.Value
	if vehicle.Value == nil {
		res.State = ConstraintUnknown
		res.Detail = "vehicle fact is unknown"
		return res
	}
	res.Actual = *vehicle.Value
	if strings.EqualFold(strings.TrimSpace(*vehicle.Value), strings.TrimSpace(*constraint.Value)) {
		res.State = ConstraintMatch
	} else {
		res.State = ConstraintConflict
	}
	return res
}

func evalIntRange(dimension string, constraint *Fact[IntRange], vehicle Fact[int], candidates []int) ConstraintResult {
	res := ConstraintResult{Dimension: dimension}
	if constraint == nil {
		res.State = ConstraintNotSupplied
		return res
	}
	if constraint.Value == nil {
		return valuelessConstraint(res, constraint.Status)
	}
	r := constraint.Value
	res.Expected = fmt.Sprintf("model year %d-%d", r.Min, r.Max)
	if vehicle.Value != nil {
		res.Actual = fmt.Sprintf("%d", *vehicle.Value)
		if r.Contains(*vehicle.Value) {
			res.State = ConstraintMatch
		} else {
			res.State = ConstraintConflict
		}
		return res
	}
	// Vehicle year unknown: if exactly one VIN candidate exists it was already
	// applied upstream; multiple candidates mean genuine ambiguity.
	res.State = ConstraintUnknown
	if len(candidates) > 1 {
		res.Detail = fmt.Sprintf("model year ambiguous: VIN candidates %v", candidates)
	} else {
		res.Detail = "vehicle model year is unknown"
	}
	return res
}

func evalProduction(constraint *Fact[ProductionRange], vehicle Fact[ProductionRange]) ConstraintResult {
	res := ConstraintResult{Dimension: "production"}
	if constraint == nil {
		res.State = ConstraintNotSupplied
		return res
	}
	if constraint.Value == nil {
		return valuelessConstraint(res, constraint.Status)
	}
	res.Expected = formatRange(constraint.Value)
	if vehicle.Value == nil {
		res.State = ConstraintUnknown
		res.Detail = "vehicle production range is unknown"
		return res
	}
	res.Actual = formatRange(vehicle.Value)
	if rangesOverlap(constraint.Value, vehicle.Value) {
		res.State = ConstraintMatch
	} else {
		res.State = ConstraintConflict
	}
	return res
}

func formatRange(r *ProductionRange) string {
	start, end := r.Start, r.End
	if start == "" {
		start = "-inf"
	}
	if end == "" {
		end = "+inf"
	}
	return start + ".." + end
}

// rangesOverlap reports whether two production ranges share any date. String
// dates in ISO form compare lexicographically.
func rangesOverlap(a, b *ProductionRange) bool {
	// a overlaps b unless a ends before b starts or b ends before a starts.
	if a.End != "" && b.Start != "" && a.End < b.Start {
		return false
	}
	if b.End != "" && a.Start != "" && b.End < a.Start {
		return false
	}
	return true
}
