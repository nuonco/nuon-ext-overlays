package selector

import (
	"fmt"
	"regexp"
	"strings"
)

// Selector represents a parsed target selector.
type Selector struct {
	// Section is the top-level config key: components, installs, sandbox, runner, policies, inputs, etc.
	Section string
	// Wildcard means select all items in an array section (e.g., components[*]).
	Wildcard bool
	// Filters are field=value predicates (e.g., name="api-server", type="helm_chart").
	Filters map[string]string
}

// selectorPattern matches: section, section[*], section[field="value"]
var selectorPattern = regexp.MustCompile(`^(\w+)(?:\[(\*|[^\]]+)\])?$`)

// filterPattern matches: field="value"
var filterPattern = regexp.MustCompile(`^(\w+)\s*=\s*"([^"]*)"$`)

// Parse parses a target string into a Selector.
func Parse(target string) (*Selector, error) {
	target = strings.TrimSpace(target)
	m := selectorPattern.FindStringSubmatch(target)
	if m == nil {
		return nil, fmt.Errorf("invalid selector: %q", target)
	}

	s := &Selector{
		Section: m[1],
		Filters: make(map[string]string),
	}

	if m[2] == "" {
		// bare section name, no brackets
		return s, nil
	}

	if m[2] == "*" {
		s.Wildcard = true
		return s, nil
	}

	// Parse filters: could be comma-separated field="value" pairs
	parts := strings.Split(m[2], ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		fm := filterPattern.FindStringSubmatch(part)
		if fm == nil {
			return nil, fmt.Errorf("invalid filter in selector %q: %q", target, part)
		}
		s.Filters[fm[1]] = fm[2]
	}

	return s, nil
}

// IsArray returns true if the section is a list type (components, installs, actions).
func (s *Selector) IsArray() bool {
	switch s.Section {
	case "components", "installs", "actions":
		return true
	}
	return false
}

// MatchesItem checks whether a map (representing a TOML table) matches the selector's filters.
func (s *Selector) MatchesItem(item map[string]any) bool {
	if s.Wildcard {
		return true
	}
	if len(s.Filters) == 0 {
		return true
	}
	for k, v := range s.Filters {
		val, ok := item[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", val) != v {
			return false
		}
	}
	return true
}
