package version

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	CLI            = "0.1.0"
	Plugin         = "0.1.0"
	Protocol       = 1
	IndexSchema    = 2
	MinCodexHost   = "0.142.5"
	SourceAdapter  = "rollout-jsonl/codex-structural/v5"
	PluginCLIRange = ">=0.1.0 <0.2.0"
)

var codexVersionPattern = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)(?:[-+][0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)

// SupportsCodexHost accepts the tested compatibility floor and newer Codex
// releases. Rollout data still has to pass the complete structural validator;
// version acceptance alone never makes a source safe to index.
func SupportsCodexHost(raw string) bool {
	got, ok := numericVersion(raw)
	if !ok {
		return false
	}
	minimum, _ := numericVersion(MinCodexHost)
	for i := range got {
		if got[i] != minimum[i] {
			return got[i] > minimum[i]
		}
	}
	// A prerelease with the same numeric core is older than the stable floor.
	return !strings.Contains(strings.SplitN(raw, "+", 2)[0], "-")
}

// IsCodexVersion reports whether raw is a well-formed Codex release string.
// Historical rollout ingestion is structure-gated, not host-version-gated:
// older sources still have to pass the complete record validator before any
// facts are normalized.
func IsCodexVersion(raw string) bool {
	_, ok := numericVersion(raw)
	return ok
}

func numericVersion(raw string) ([3]int, bool) {
	match := codexVersionPattern.FindStringSubmatch(raw)
	if len(match) != 4 {
		return [3]int{}, false
	}
	var parsed [3]int
	for i := range parsed {
		value, err := strconv.Atoi(match[i+1])
		if err != nil {
			return [3]int{}, false
		}
		parsed[i] = value
	}
	return parsed, true
}
