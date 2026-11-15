package vehicle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ConfigSchemaVersion versions the component configuration and vehicle
// configuration document formats independently of the vehicle profile schema.
const ConfigSchemaVersion = "1.0.0"

// EngineConfig describes an engine family/variant as structured facts. Fields
// without evidence remain unknown; a configuration never asserts details that
// have not been verified.
type EngineConfig struct {
	SchemaVersion      string                 `json:"schemaVersion"`
	ID                 string                 `json:"id"`
	DisplayName        string                 `json:"displayName"`
	Make               Fact[string]           `json:"make"`
	Family             Fact[string]           `json:"family"`
	DisplacementLiters Fact[float64]          `json:"displacementLiters"`
	Cylinders          Fact[int]              `json:"cylinders"`
	Aspiration         Fact[string]           `json:"aspiration"`
	FuelSystem         Fact[string]           `json:"fuelSystem"`
	Valvetrain         Fact[string]           `json:"valvetrain"`
	Production         *Fact[ProductionRange] `json:"production,omitempty"`
	Notes              string                 `json:"notes,omitempty"`
}

// UnmarshalJSON rejects an explicit null production, which the schema forbids
// but encoding/json would otherwise decode as an omitted field.
func (c *EngineConfig) UnmarshalJSON(data []byte) error {
	type alias EngineConfig
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, v := range raw {
		// encoding/json matches field names case-insensitively.
		if strings.EqualFold(key, "production") && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return errors.New("production must be an object when present")
		}
	}
	decoded := alias(*c)
	if err := decodeStrict(data, &decoded); err != nil {
		return err
	}
	*c = EngineConfig(decoded)
	return nil
}

// TransmissionConfig describes a transmission type/variant. The specific
// gearbox family must remain unknown until verified; type alone does not
// identify a transmission.
type TransmissionConfig struct {
	SchemaVersion string       `json:"schemaVersion"`
	ID            string       `json:"id"`
	DisplayName   string       `json:"displayName"`
	Type          Fact[string] `json:"type"`
	Family        Fact[string] `json:"family"`
	GearCount     Fact[int]    `json:"gearCount"`
	Notes         string       `json:"notes,omitempty"`
}

// DrivetrainConfig describes a drivetrain layout/variant.
type DrivetrainConfig struct {
	SchemaVersion string       `json:"schemaVersion"`
	ID            string       `json:"id"`
	DisplayName   string       `json:"displayName"`
	Layout        Fact[string] `json:"layout"`
	Notes         string       `json:"notes,omitempty"`
}

// TrimConfig describes a body/trim variant: factory body code, body style, and
// trim level for a market and model.
type TrimConfig struct {
	SchemaVersion string       `json:"schemaVersion"`
	ID            string       `json:"id"`
	DisplayName   string       `json:"displayName"`
	Market        Fact[string] `json:"market"`
	Model         Fact[string] `json:"model"`
	BodyCode      Fact[string] `json:"bodyCode"`
	BodyStyle     Fact[string] `json:"bodyStyle"`
	TrimLevel     Fact[string] `json:"trimLevel"`
	Notes         string       `json:"notes,omitempty"`
}

// VehicleConfiguration binds a vehicle profile to component configuration
// documents by stable ID. Each reference is a fact: a confirmed or inferred
// reference requires evidence, and unknown or unsupported references carry no
// value. Component details live only in the referenced documents; the binding
// never duplicates them.
type VehicleConfiguration struct {
	SchemaVersion string       `json:"schemaVersion"`
	ID            string       `json:"id"`
	DisplayName   string       `json:"displayName"`
	Profile       Fact[string] `json:"profile"`
	Engine        Fact[string] `json:"engine"`
	Transmission  Fact[string] `json:"transmission"`
	Drivetrain    Fact[string] `json:"drivetrain"`
	Body          Fact[string] `json:"body"`
	Trim          Fact[string] `json:"trim"`
	Notes         string       `json:"notes,omitempty"`
}

var transmissionTypes = map[string]bool{
	"manual": true, "automatic": true, "cvt": true, "dct": true, "other": true,
}

var drivetrainLayouts = map[string]bool{
	"fwd": true, "rwd": true, "awd": true, "4wd": true, "other": true,
}

func checkHeader(schemaVersion, id, displayName string) []error {
	var problems []error
	if schemaVersion != ConfigSchemaVersion {
		problems = append(problems, fmt.Errorf("schemaVersion must be %q", ConfigSchemaVersion))
	}
	if strings.TrimSpace(id) == "" {
		problems = append(problems, errors.New("id is required"))
	} else if !profileIDPattern.MatchString(id) {
		problems = append(problems, fmt.Errorf("id must match %s", profileIDPattern.String()))
	}
	if strings.TrimSpace(displayName) == "" {
		problems = append(problems, errors.New("displayName is required"))
	}
	return problems
}

func checkStringFacts(fields map[string]Fact[string]) []error {
	var problems []error
	for name, fact := range fields {
		if err := validateStringFact(name, fact); err != nil {
			problems = append(problems, err)
		}
	}
	return problems
}

func checkCountFact(name string, fact Fact[int], min, max int) error {
	if err := validateFact(name, fact); err != nil {
		return err
	}
	if fact.Value != nil && (*fact.Value < min || *fact.Value > max) {
		return fmt.Errorf("%s: value must be between %d and %d", name, min, max)
	}
	return nil
}

func checkEnumFact(name, value string, allowed map[string]bool) error {
	if value != "" && !allowed[value] {
		return fmt.Errorf("%s: unsupported value %q", name, value)
	}
	return nil
}

func (c EngineConfig) Validate() error {
	problems := checkHeader(c.SchemaVersion, c.ID, c.DisplayName)
	problems = append(problems, checkStringFacts(map[string]Fact[string]{
		"make": c.Make, "family": c.Family,
		"aspiration": c.Aspiration, "fuelSystem": c.FuelSystem, "valvetrain": c.Valvetrain,
	})...)
	if err := validateFact("displacementLiters", c.DisplacementLiters); err != nil {
		problems = append(problems, err)
	}
	if c.DisplacementLiters.Value != nil && *c.DisplacementLiters.Value <= 0 {
		problems = append(problems, errors.New("displacementLiters: value must be greater than 0"))
	}
	if err := checkCountFact("cylinders", c.Cylinders, 1, 16); err != nil {
		problems = append(problems, err)
	}
	if c.Production != nil {
		if err := validateFact("production", *c.Production); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func (c TransmissionConfig) Validate() error {
	problems := checkHeader(c.SchemaVersion, c.ID, c.DisplayName)
	problems = append(problems, checkStringFacts(map[string]Fact[string]{
		"type": c.Type, "family": c.Family,
	})...)
	if c.Type.Value != nil {
		if err := checkEnumFact("type", *c.Type.Value, transmissionTypes); err != nil {
			problems = append(problems, err)
		}
	}
	if err := checkCountFact("gearCount", c.GearCount, 1, 12); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

func (c DrivetrainConfig) Validate() error {
	problems := checkHeader(c.SchemaVersion, c.ID, c.DisplayName)
	problems = append(problems, checkStringFacts(map[string]Fact[string]{"layout": c.Layout})...)
	if c.Layout.Value != nil {
		if err := checkEnumFact("layout", *c.Layout.Value, drivetrainLayouts); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func (c TrimConfig) Validate() error {
	problems := checkHeader(c.SchemaVersion, c.ID, c.DisplayName)
	problems = append(problems, checkStringFacts(map[string]Fact[string]{
		"market": c.Market, "model": c.Model, "bodyCode": c.BodyCode,
		"bodyStyle": c.BodyStyle, "trimLevel": c.TrimLevel,
	})...)
	return errors.Join(problems...)
}

func (c VehicleConfiguration) Validate() error {
	problems := checkHeader(c.SchemaVersion, c.ID, c.DisplayName)
	problems = append(problems, checkStringFacts(map[string]Fact[string]{
		"profile": c.Profile, "engine": c.Engine, "transmission": c.Transmission,
		"drivetrain": c.Drivetrain, "body": c.Body, "trim": c.Trim,
	})...)
	return errors.Join(problems...)
}

// Registry stores validated vehicle profiles, component configurations, and
// vehicle configurations by stable ID so references can be resolved without
// duplicating factory data.
type Registry struct {
	Profiles       map[string]VehicleProfile
	Engines        map[string]EngineConfig
	Transmissions  map[string]TransmissionConfig
	Drivetrains    map[string]DrivetrainConfig
	Trims          map[string]TrimConfig
	Configurations map[string]VehicleConfiguration
	Hardware       map[string]HardwareProfile
	Catalogs       map[string]CompatibilityCatalog
}

func NewRegistry() *Registry {
	return &Registry{
		Profiles:       map[string]VehicleProfile{},
		Engines:        map[string]EngineConfig{},
		Transmissions:  map[string]TransmissionConfig{},
		Drivetrains:    map[string]DrivetrainConfig{},
		Trims:          map[string]TrimConfig{},
		Configurations: map[string]VehicleConfiguration{},
		Hardware:       map[string]HardwareProfile{},
		Catalogs:       map[string]CompatibilityCatalog{},
	}
}

func register[T any](m map[string]T, id string, doc T, validate func(T) error) error {
	if err := validate(doc); err != nil {
		return err
	}
	if _, exists := m[id]; exists {
		return fmt.Errorf("duplicate definition id %q", id)
	}
	m[id] = doc
	return nil
}

func (r *Registry) RegisterProfile(p VehicleProfile) error {
	return register(r.Profiles, p.ID, p, func(d VehicleProfile) error { return d.Validate() })
}

func (r *Registry) RegisterEngine(c EngineConfig) error {
	return register(r.Engines, c.ID, c, func(d EngineConfig) error { return d.Validate() })
}

func (r *Registry) RegisterTransmission(c TransmissionConfig) error {
	return register(r.Transmissions, c.ID, c, func(d TransmissionConfig) error { return d.Validate() })
}

func (r *Registry) RegisterDrivetrain(c DrivetrainConfig) error {
	return register(r.Drivetrains, c.ID, c, func(d DrivetrainConfig) error { return d.Validate() })
}

func (r *Registry) RegisterTrim(c TrimConfig) error {
	return register(r.Trims, c.ID, c, func(d TrimConfig) error { return d.Validate() })
}

func (r *Registry) RegisterConfiguration(c VehicleConfiguration) error {
	return register(r.Configurations, c.ID, c, func(d VehicleConfiguration) error { return d.Validate() })
}

// Resolve checks that every valued reference in c resolves to a registered
// definition of the matching kind. Unknown and unsupported references are
// explicit and skipped; a valued reference with no registered definition is an
// error so configuration differences surface as data problems, not silent
// assumptions in diagnostic code.
func (r *Registry) Resolve(c VehicleConfiguration) error {
	var problems []error
	check := func(name string, fact Fact[string], exists func(string) bool) {
		if fact.Value == nil {
			return
		}
		if !exists(*fact.Value) {
			problems = append(problems, fmt.Errorf("%s: no registered %s definition %q", c.ID, name, *fact.Value))
		}
	}
	check("profile", c.Profile, func(id string) bool { _, ok := r.Profiles[id]; return ok })
	check("engine", c.Engine, func(id string) bool { _, ok := r.Engines[id]; return ok })
	check("transmission", c.Transmission, func(id string) bool { _, ok := r.Transmissions[id]; return ok })
	check("drivetrain", c.Drivetrain, func(id string) bool { _, ok := r.Drivetrains[id]; return ok })
	check("body", c.Body, func(id string) bool { _, ok := r.Trims[id]; return ok })
	check("trim", c.Trim, func(id string) bool { _, ok := r.Trims[id]; return ok })
	return errors.Join(problems...)
}
