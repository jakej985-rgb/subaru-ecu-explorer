package vehicle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const CurrentSchemaVersion = "1.0.0"

var profileIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type FactStatus string

const (
	StatusConfirmed   FactStatus = "confirmed"
	StatusInferred    FactStatus = "inferred"
	StatusUnknown     FactStatus = "unknown"
	StatusUnsupported FactStatus = "unsupported"
)

type Evidence struct {
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
	Reference   string `json:"reference,omitempty"`
}

type Fact[T any] struct {
	Value    *T         `json:"value,omitempty"`
	Status   FactStatus `json:"status"`
	Evidence []Evidence `json:"evidence,omitempty"`
	Notes    string     `json:"notes,omitempty"`
}

type ProductionRange struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

type VehicleProfile struct {
	SchemaVersion string                 `json:"schemaVersion"`
	ID            string                 `json:"id"`
	DisplayName   string                 `json:"displayName"`
	ModelYear     Fact[int]               `json:"modelYear"`
	Market        Fact[string]            `json:"market"`
	Make          Fact[string]            `json:"make"`
	Model         Fact[string]            `json:"model"`
	Body          Fact[string]            `json:"body"`
	Trim          Fact[string]            `json:"trim"`
	Engine        Fact[string]            `json:"engine"`
	Transmission  Fact[string]            `json:"transmission"`
	Drivetrain    Fact[string]            `json:"drivetrain"`
	Production    *Fact[ProductionRange]  `json:"production,omitempty"`
	Identifiers   map[string]Fact[string] `json:"identifiers,omitempty"`

	// productionNull records an explicit JSON null, which the schema rejects.
	productionNull bool
}

func (p *VehicleProfile) UnmarshalJSON(data []byte) error {
	type alias VehicleProfile
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = VehicleProfile(decoded)
	if v, ok := raw["production"]; ok && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		p.productionNull = true
	}
	return nil
}

func (p VehicleProfile) Validate() error {
	var problems []error
	if p.SchemaVersion != CurrentSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %q", CurrentSchemaVersion))
	}
	if strings.TrimSpace(p.ID) == "" {
		problems = append(problems, errors.New("id is required"))
	} else if !profileIDPattern.MatchString(p.ID) {
		problems = append(problems, fmt.Errorf("id must match %s", profileIDPattern.String()))
	}
	if strings.TrimSpace(p.DisplayName) == "" {
		problems = append(problems, errors.New("displayName is required"))
	}

	checks := []struct {
		name string
		fact Fact[string]
	}{
		{"market", p.Market}, {"make", p.Make}, {"model", p.Model},
		{"body", p.Body}, {"trim", p.Trim}, {"engine", p.Engine},
		{"transmission", p.Transmission}, {"drivetrain", p.Drivetrain},
	}
	for _, check := range checks {
		if err := validateStringFact(check.name, check.fact); err != nil {
			problems = append(problems, err)
		}
	}
	if err := validateFact("modelYear", p.ModelYear); err != nil {
		problems = append(problems, err)
	}
	if p.ModelYear.Value != nil && (*p.ModelYear.Value < 1900 || *p.ModelYear.Value > 2100) {
		problems = append(problems, errors.New("modelYear: value must be between 1900 and 2100"))
	}
	if p.productionNull {
		problems = append(problems, errors.New("production must be an object when present"))
	} else if p.Production != nil {
		if err := validateFact("production", *p.Production); err != nil {
			problems = append(problems, err)
		}
	}
	for key, fact := range p.Identifiers {
		if strings.TrimSpace(key) == "" {
			problems = append(problems, errors.New("identifier keys cannot be blank"))
			continue
		}
		if err := validateStringFact("identifiers."+key, fact); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func validateStringFact(name string, fact Fact[string]) error {
	if err := validateFact(name, fact); err != nil {
		return err
	}
	if fact.Value != nil && *fact.Value == "" {
		return fmt.Errorf("%s: value must not be empty", name)
	}
	return nil
}

func validateFact[T any](name string, fact Fact[T]) error {
	switch fact.Status {
	case StatusConfirmed, StatusInferred:
		if fact.Value == nil {
			return fmt.Errorf("%s: status %q requires a value", name, fact.Status)
		}
		if len(fact.Evidence) == 0 {
			return fmt.Errorf("%s: status %q requires at least one evidence item", name, fact.Status)
		}
	case StatusUnknown, StatusUnsupported:
		if fact.Value != nil {
			return fmt.Errorf("%s: status %q must not contain a value", name, fact.Status)
		}
	default:
		return fmt.Errorf("%s: invalid fact status %q", name, fact.Status)
	}
	for i, item := range fact.Evidence {
		if strings.TrimSpace(item.Source) == "" {
			return fmt.Errorf("%s: evidence[%d].source is required", name, i)
		}
	}
	return nil
}
