package vehicle

import (
	"errors"
	"fmt"
	"strings"
)

// InterfaceSchemaVersion versions the diagnostic interface profile format.
const InterfaceSchemaVersion = "1.0.0"

// InterfaceKind identifies a diagnostic interface/adapter path. These are
// communication paths to the vehicle, not installed vehicle hardware, and
// compatibility with an interface never implies module fitment.
type InterfaceKind string

const (
	// InterfaceELM327 is a generic ELM327-compatible OBD-II adapter path.
	InterfaceELM327 InterfaceKind = "elm327-odbii"
	// InterfaceSubaruSSM is the Subaru SSM diagnostic protocol path.
	InterfaceSubaruSSM InterfaceKind = "subaru-ssm"
	// InterfaceArduinoSerial is the custom Arduino Mega acquisition path over
	// USB serial with framed, versioned messages.
	InterfaceArduinoSerial InterfaceKind = "arduino-serial"
)

// InterfaceProfile describes a diagnostic interface path: its transport,
// protocol family, and read-only posture. It is evaluated separately from the
// hardware compatibility catalog — an interface profile says nothing about
// which ECUs or modules are installed.
type InterfaceProfile struct {
	SchemaVersion string        `json:"schemaVersion"`
	ID            string        `json:"id"`
	DisplayName   string        `json:"displayName"`
	Kind          InterfaceKind `json:"kind"`
	Transport     Fact[string]  `json:"transport"`
	Protocol      Fact[string]  `json:"protocol"`
	ReadOnly      Fact[bool]    `json:"readOnly"`
	Notes         string        `json:"notes,omitempty"`
}

var interfaceKinds = map[InterfaceKind]bool{
	InterfaceELM327:        true,
	InterfaceSubaruSSM:     true,
	InterfaceArduinoSerial: true,
}

func (p InterfaceProfile) Validate() error {
	var problems []error
	if p.SchemaVersion != InterfaceSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %q", InterfaceSchemaVersion))
	}
	if strings.TrimSpace(p.ID) == "" {
		problems = append(problems, errors.New("id is required"))
	} else if !profileIDPattern.MatchString(p.ID) {
		problems = append(problems, fmt.Errorf("id must match %s", profileIDPattern.String()))
	}
	if strings.TrimSpace(p.DisplayName) == "" {
		problems = append(problems, errors.New("displayName is required"))
	}
	if !interfaceKinds[p.Kind] {
		problems = append(problems, fmt.Errorf("kind: unsupported interface kind %q", p.Kind))
	}
	problems = append(problems, checkStringFacts(map[string]Fact[string]{
		"transport": p.Transport, "protocol": p.Protocol,
	})...)
	if err := validateFact("readOnly", p.ReadOnly); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

// RegisterInterface validates and stores an interface profile.
func (r *Registry) RegisterInterface(p InterfaceProfile) error {
	return register(r.Interfaces, p.ID, p, func(d InterfaceProfile) error { return d.Validate() })
}
