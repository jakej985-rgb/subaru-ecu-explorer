package elm327

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jakej985-rgb/subaru-ecu-explorer/internal/protocol"
)

// InitSteps is the conservative ELM327 initialization sequence for
// read-only OBD-II work: reset, echo off, headers off, spaces off,
// automatic protocol. Order matters — ATZ must come first.
var InitSteps = []string{"Z", "E0", "H0", "S0", "SP0"}

// Identify runs the initialization sequence then ATI to capture adapter
// identity. Each step must be acknowledged; the first failure aborts with
// a descriptive error. onMsg, when non-nil, observes every exchanged
// message for logging/evidence.
func Initialize(ctx context.Context, s *protocol.Session, onMsg func(protocol.Message)) error {
	for _, step := range InitSteps {
		resp, err := s.Transact(ctx, AT(step))
		if onMsg != nil {
			for _, m := range resp.Messages {
				onMsg(m)
			}
		}
		if err != nil {
			return fmt.Errorf("elm327 init %s: %w", step, err)
		}
		if e := lastError(resp); e != nil {
			return fmt.Errorf("elm327 init %s: %w", step, e)
		}
	}
	return nil
}

// AdapterInfo is the detected ELM327 identity and capabilities.
type AdapterInfo struct {
	// RawVersion is the banner text (e.g. "ELM327 v1.5").
	RawVersion string
	// Version is the parsed dotted version when recognizable.
	Version string
	// Clone reports likely-clone firmware heuristically (v1.4/v1.5
	// claiming STN features is impossible; bare "ELM327" without version
	// is suspicious).
	Clone bool
	// Protocols lists protocol names reported by ATDP, when queried.
	Protocols string
}

var versionRe = regexp.MustCompile(`(?i)(?:elm327|v|version)\s*([0-9]+(?:\.[0-9]+)*)`)

// Identify runs Initialize (unless alreadyDone) then ATI (+ optional
// ATDP) and returns parsed adapter identity.
func Identify(ctx context.Context, s *protocol.Session, queryProtocol bool) (*AdapterInfo, error) {
	if err := Initialize(ctx, s, nil); err != nil {
		return nil, err
	}
	info := &AdapterInfo{}
	resp, err := s.Transact(ctx, AT("I"))
	if err != nil {
		return nil, fmt.Errorf("elm327 ATI: %w", err)
	}
	for _, m := range resp.Answers {
		if KindOf(m) == "banner" {
			info.RawVersion = string(m.Payload)
		}
	}
	if info.RawVersion == "" {
		// Some clones answer ATI with a bare version line classified as
		// data or ok-adjacent text; fall back to any text payload.
		for _, m := range resp.Answers {
			if len(m.Payload) > 0 && !IsData(m) {
				info.RawVersion = string(m.Payload)
				break
			}
		}
	}
	if mm := versionRe.FindStringSubmatch(info.RawVersion); len(mm) > 1 {
		info.Version = mm[1]
	}
	info.Clone = detectClone(info.RawVersion, info.Version)

	if queryProtocol {
		pr, err := s.Transact(ctx, AT("DP"))
		if err == nil {
			for _, m := range pr.Answers {
				if len(m.Payload) > 0 {
					info.Protocols = string(m.Payload)
				}
			}
		}
	}
	return info, nil
}

// detectClone applies conservative heuristics. It can only *suspect* a
// clone — it never certifies compatibility.
func detectClone(raw, version string) bool {
	l := strings.ToLower(raw)
	if strings.Contains(l, "clone") || strings.Contains(l, "vlinker") ||
		strings.Contains(l, "v-volt") || strings.Contains(l, "scanmaster") {
		return true
	}
	// A bare "ELM327" with no version number is suspicious.
	if version == "" && strings.Contains(l, "elm327") {
		return true
	}
	return false
}

// lastError returns the typed error of the last answer message, if any.
func lastError(resp protocol.Response) error {
	for i := len(resp.Answers) - 1; i >= 0; i-- {
		if e := ErrorOf(resp.Answers[i]); e != nil {
			return e
		}
	}
	return nil
}

// CommandTimeout is the suggested per-request timeout for AT commands;
// OBD requests searching for ECUs need longer (see Request.Timeout).
const CommandTimeout = 2 * time.Second

// ParseVersion extracts a dotted version from arbitrary banner text.
func ParseVersion(s string) string {
	if mm := versionRe.FindStringSubmatch(s); len(mm) > 1 {
		return mm[1]
	}
	return ""
}

// AtoiVersion parses "1.5"-style versions into major/minor.
func AtoiVersion(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return maj, min, true
}
