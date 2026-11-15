package vehicle

import (
	"errors"
	"fmt"
	"strings"
)

// IntRange is an inclusive integer range. A zero Min or Max means unbounded on
// that side; both zero is invalid.
type IntRange struct {
	Min int `json:"min,omitempty"`
	Max int `json:"max,omitempty"`
}

// Contains reports whether year falls inside the range.
func (r IntRange) Contains(year int) bool {
	if r.Min != 0 && year < r.Min {
		return false
	}
	if r.Max != 0 && year > r.Max {
		return false
	}
	return true
}

// Applicability holds the structured fitment constraints a catalog entry
// places on a vehicle. Each dimension is optional: an absent constraint does
// not restrict fitment. Present constraints are facts, so their own status
// and evidence rules apply.
type Applicability struct {
	ModelYear    *Fact[IntRange]        `json:"modelYear,omitempty"`
	Market       *Fact[string]          `json:"market,omitempty"`
	Model        *Fact[string]          `json:"model,omitempty"`
	Body         *Fact[string]          `json:"body,omitempty"`
	Engine       *Fact[string]          `json:"engine,omitempty"`
	Transmission *Fact[string]          `json:"transmission,omitempty"`
	Drivetrain   *Fact[string]          `json:"drivetrain,omitempty"`
	Production   *Fact[ProductionRange] `json:"production,omitempty"`
	// Options constrains factory options by name. Vehicle option data is not
	// yet modeled, so any supplied option constraint evaluates as unknown.
	Options map[string]Fact[string] `json:"options,omitempty"`
}

// Validate checks every supplied constraint fact.
func (a Applicability) Validate() error {
	var problems []error
	stringDims := []struct {
		name string
		fact *Fact[string]
	}{
		{"market", a.Market}, {"model", a.Model}, {"body", a.Body},
		{"engine", a.Engine}, {"transmission", a.Transmission}, {"drivetrain", a.Drivetrain},
	}
	for _, d := range stringDims {
		if d.fact == nil {
			continue
		}
		if err := validateStringFact(d.name, *d.fact); err != nil {
			problems = append(problems, err)
		}
	}
	if a.ModelYear != nil {
		if err := validateFact("modelYear", *a.ModelYear); err != nil {
			problems = append(problems, err)
		} else if a.ModelYear.Value != nil && a.ModelYear.Value.Min == 0 && a.ModelYear.Value.Max == 0 {
			problems = append(problems, errors.New("modelYear: range requires at least one of min/max"))
		} else if a.ModelYear.Value != nil && a.ModelYear.Value.Max != 0 && a.ModelYear.Value.Min > a.ModelYear.Value.Max {
			problems = append(problems, fmt.Errorf("modelYear: min %d exceeds max %d", a.ModelYear.Value.Min, a.ModelYear.Value.Max))
		}
	}
	if a.Production != nil {
		if err := validateFact("production", *a.Production); err != nil {
			problems = append(problems, err)
		}
	}
	for name, fact := range a.Options {
		if strings.TrimSpace(name) == "" {
			problems = append(problems, errors.New("options: blank option name"))
			continue
		}
		if err := validateStringFact("options."+name, fact); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}
