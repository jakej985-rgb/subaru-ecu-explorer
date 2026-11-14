package vehicle

import (
	"errors"
	"fmt"
	"strings"
)

const CurrentSchemaVersion = "1.0.0"

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
	Production    Fact[ProductionRange]   `json:"production,omitempty"`
	Identifiers   map[string]Fact[string] `json:"identifiers,omitempty"`
}

func (p VehicleProfile) Validate() error {
	var problems []error
	if p.SchemaVersion != CurrentSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %q", CurrentSchemaVersion))
	}
	if strings.TrimSpace(p.ID) == "" {
		problems = append(problems, errors.New("id is required"))
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
		if err := validateFact(check.name, check.fact); err != nil {
			problems = append(problems, err)
		}
	}
	if err := validateFact("modelYear", p.ModelYear); err != nil {
		problems = append(problems, err)
	}
	if err := validateFact("production", p.Production); err != nil {
		problems = append(problems, err)
	}
	for key, fact := range p.Identifiers {
		if strings.TrimSpace(key) == "" {
			problems = append(problems, errors.New("identifier keys cannot be blank"))
			continue
		}
		if err := validateFact("identifiers."+key, fact); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
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
