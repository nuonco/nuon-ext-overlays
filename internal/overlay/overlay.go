package overlay

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Strategy controls how a patch is applied.
type Strategy string

const (
	StrategyMerge   Strategy = "merge"
	StrategyReplace Strategy = "replace"
	StrategyDelete  Strategy = "delete"
)

// Overlay is the top-level overlay.toml structure.
type Overlay struct {
	Version string   `toml:"version"`
	Bases   []string `toml:"bases,omitempty"`
	Patches []Patch  `toml:"patches"`
}

// Patch describes a single overlay patch targeting a config section.
type Patch struct {
	Target   string         `toml:"target"`
	Strategy Strategy       `toml:"strategy,omitempty"`
	Set      map[string]any `toml:"set,omitempty"`
	Value    map[string]any `toml:"value,omitempty"`
}

// ParseFile reads and parses an overlay.toml file.
func ParseFile(path string) (*Overlay, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading overlay file: %w", err)
	}
	return Parse(data)
}

// Parse parses overlay TOML bytes.
func Parse(data []byte) (*Overlay, error) {
	var o Overlay
	if err := toml.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("parsing overlay: %w", err)
	}

	if o.Version == "" {
		o.Version = "1"
	}

	for i := range o.Patches {
		if o.Patches[i].Strategy == "" {
			o.Patches[i].Strategy = StrategyMerge
		}
	}

	return &o, nil
}

// Validate checks that the overlay is well-formed.
func (o *Overlay) Validate() error {
	if len(o.Bases) == 0 && len(o.Patches) == 0 {
		return fmt.Errorf("overlay must have at least one base or patch")
	}
	for i, p := range o.Patches {
		if p.Target == "" {
			return fmt.Errorf("patch %d: target is required", i)
		}
		switch p.Strategy {
		case StrategyMerge:
			if len(p.Set) == 0 {
				return fmt.Errorf("patch %d: merge strategy requires 'set' fields", i)
			}
		case StrategyReplace:
			if len(p.Value) == 0 {
				return fmt.Errorf("patch %d: replace strategy requires 'value' fields", i)
			}
		case StrategyDelete:
			// no additional fields needed
		default:
			return fmt.Errorf("patch %d: unknown strategy %q (must be merge, replace, or delete)", i, p.Strategy)
		}
	}
	return nil
}
