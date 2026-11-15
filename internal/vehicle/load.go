package vehicle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// definitionKinds maps data/definitions subdirectories to the document type
// they contain. Directories not listed here are ignored.
var definitionKinds = map[string]string{
	"vehicles":       "profile",
	"engines":        "engine",
	"transmissions":  "transmission",
	"drivetrains":    "drivetrain",
	"trims":          "trim",
	"configurations": "configuration",
	"hardware":       "hardware",
	"catalogs":       "catalog",
	"interfaces":     "interface",
	"connectors":     "connector",
}

// LoadResult records the outcome for one definition file.
type LoadResult struct {
	Path string
	Kind string
	Err  error
}

// LoadReport summarizes a LoadDefinitions run.
type LoadReport struct {
	Results []LoadResult
}

// OK reports whether every discovered definition file loaded and validated.
func (r LoadReport) OK() bool {
	for _, res := range r.Results {
		if res.Err != nil {
			return false
		}
	}
	return true
}

// LoadDefinitions walks root (for example data/definitions), strictly decodes
// every JSON file in a known kind directory, validates it, and registers it
// in r. Files that fail are recorded in the report; loading continues so all
// problems surface in one pass. Unknown directories are skipped.
func LoadDefinitions(root string, r *Registry) LoadReport {
	var report LoadReport
	entries, err := os.ReadDir(root)
	if err != nil {
		report.Results = append(report.Results, LoadResult{Path: root, Err: err})
		return report
	}
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		kind, ok := definitionKinds[dir.Name()]
		if !ok {
			continue
		}
		dirPath := filepath.Join(root, dir.Name())
		files, err := os.ReadDir(dirPath)
		if err != nil {
			report.Results = append(report.Results, LoadResult{Path: dirPath, Kind: kind, Err: err})
			continue
		}
		var names []string
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
				names = append(names, f.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(dirPath, name)
			res := LoadResult{Path: path, Kind: kind}
			if err := loadDefinitionFile(path, kind, r); err != nil {
				res.Err = err
			}
			report.Results = append(report.Results, res)
		}
	}
	return report
}

// loadDefinitionFile strictly decodes one file into its document type and
// registers it. Strict decoding rejects unknown top-level fields, matching
// the additionalProperties:false rule in the JSON schemas.
func loadDefinitionFile(path, kind string, r *Registry) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decode := func(out any) error { return decodeStrict(data, out) }
	var regErr error
	switch kind {
	case "profile":
		var d VehicleProfile
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterProfile(d)
	case "engine":
		var d EngineConfig
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterEngine(d)
	case "transmission":
		var d TransmissionConfig
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterTransmission(d)
	case "drivetrain":
		var d DrivetrainConfig
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterDrivetrain(d)
	case "trim":
		var d TrimConfig
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterTrim(d)
	case "configuration":
		var d VehicleConfiguration
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterConfiguration(d)
	case "hardware":
		var d HardwareProfile
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterHardwareProfile(d)
	case "catalog":
		var d CompatibilityCatalog
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterCatalog(d)
	case "interface":
		var d InterfaceProfile
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterInterface(d)
	case "connector":
		var d Connector
		if err := decode(&d); err != nil {
			return err
		}
		regErr = r.RegisterConnector(d)
	default:
		return fmt.Errorf("unknown definition kind %q", kind)
	}
	return regErr
}

// decodeStrict decodes exactly one JSON value into out, rejecting unknown
// fields and any trailing content.
func decodeStrict(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("unexpected trailing content after JSON document")
	}
	return nil
}

// ValidateRegistry runs cross-document reference checks after loading: every
// configuration's valued references must resolve, and every catalog's valued
// configuration/hardware references must resolve.
func ValidateRegistry(r *Registry) error {
	var problems []error
	for _, cfg := range r.Configurations {
		if err := r.Resolve(cfg); err != nil {
			problems = append(problems, fmt.Errorf("configuration %s: %w", cfg.ID, err))
		}
	}
	for _, cat := range r.Catalogs {
		if err := r.ResolveCatalog(cat); err != nil {
			problems = append(problems, fmt.Errorf("catalog %s: %w", cat.ID, err))
		}
	}
	return errors.Join(problems...)
}
