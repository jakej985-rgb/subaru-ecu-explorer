package vehicle

import (
	"errors"
	"fmt"
	"strings"
)

// ConnectorSchemaVersion versions the connector pin-map document format.
const ConnectorSchemaVersion = "1.0.0"

// ConnectorKind identifies a physical vehicle connector type.
type ConnectorKind string

const (
	// ConnectorOBDIIDLC is the standard 16-position OBD-II data link connector.
	ConnectorOBDIIDLC ConnectorKind = "obd-ii-dlc"
	// ConnectorDedicatedDiagnostic is a manufacturer-dedicated diagnostic
	// connector whose pinout is vehicle- and system-specific.
	ConnectorDedicatedDiagnostic ConnectorKind = "dedicated-diagnostic"
)

var connectorKinds = map[ConnectorKind]bool{
	ConnectorOBDIIDLC:            true,
	ConnectorDedicatedDiagnostic: true,
}

// ConnectorPin describes one position on a connector. Function is a fact:
// the generic role documented by a standard or factory source. Whether the
// pin is populated and in use on a specific vehicle is a separate question
// recorded in Notes — it is never inferred from the standard assignment.
type ConnectorPin struct {
	Number   int          `json:"number"`
	Function Fact[string] `json:"function"`
	Notes    string       `json:"notes,omitempty"`
}

// Connector is a pin map for one physical vehicle connector. Pin functions
// carry provenance; unknown positions must stay unknown rather than being
// guessed from wire color, connector position, or generic OBD-II tables.
type Connector struct {
	SchemaVersion string         `json:"schemaVersion"`
	ID            string         `json:"id"`
	DisplayName   string         `json:"displayName"`
	Kind          ConnectorKind  `json:"kind"`
	PinCount      int            `json:"pinCount"`
	Pins          []ConnectorPin `json:"pins"`
	Notes         string         `json:"notes,omitempty"`
}

func (c Connector) Validate() error {
	var problems []error
	if c.SchemaVersion != ConnectorSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %q", ConnectorSchemaVersion))
	}
	if strings.TrimSpace(c.ID) == "" {
		problems = append(problems, errors.New("id is required"))
	} else if !profileIDPattern.MatchString(c.ID) {
		problems = append(problems, fmt.Errorf("id must match %s", profileIDPattern.String()))
	}
	if strings.TrimSpace(c.DisplayName) == "" {
		problems = append(problems, errors.New("displayName is required"))
	}
	if !connectorKinds[c.Kind] {
		problems = append(problems, fmt.Errorf("kind: unsupported connector kind %q", c.Kind))
	}
	if c.PinCount <= 0 {
		problems = append(problems, errors.New("pinCount must be positive"))
	}
	if len(c.Pins) == 0 {
		problems = append(problems, errors.New("pins must contain at least one pin"))
	}
	seen := map[int]bool{}
	for i, p := range c.Pins {
		name := fmt.Sprintf("pins[%d]", i)
		if p.Number < 1 || (c.PinCount > 0 && p.Number > c.PinCount) {
			problems = append(problems, fmt.Errorf("%s: pin number %d out of range 1-%d", name, p.Number, c.PinCount))
		}
		if seen[p.Number] {
			problems = append(problems, fmt.Errorf("%s: duplicate pin number %d", name, p.Number))
		}
		seen[p.Number] = true
		if err := validateStringFact(name+".function", p.Function); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

// RegisterConnector validates and stores a connector pin map.
func (r *Registry) RegisterConnector(c Connector) error {
	return register(r.Connectors, c.ID, c, func(d Connector) error { return d.Validate() })
}
