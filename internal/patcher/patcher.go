package patcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nuonco/nuon-ext-overlays/internal/overlay"
	"github.com/nuonco/nuon-ext-overlays/internal/selector"
)

// ConfigDir represents all TOML files in a Nuon app config directory.
// Keys are relative file paths, values are the parsed TOML data.
type ConfigDir map[string]map[string]any

// LoadConfigDir reads all .toml files from a directory tree into memory.
func LoadConfigDir(dir string) (ConfigDir, error) {
	cfg := make(ConfigDir)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".toml") {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}

		var doc map[string]any
		if err := toml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing %s: %w", rel, err)
		}

		cfg[rel] = doc
		return nil
	})
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// WriteConfigDir writes all TOML files to a destination directory.
func WriteConfigDir(cfg ConfigDir, destDir string) error {
	for relPath, doc := range cfg {
		fullPath := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", relPath, err)
		}

		f, err := os.Create(fullPath)
		if err != nil {
			return fmt.Errorf("creating %s: %w", relPath, err)
		}

		enc := toml.NewEncoder(f)
		if err := enc.Encode(doc); err != nil {
			f.Close()
			return fmt.Errorf("encoding %s: %w", relPath, err)
		}
		f.Close()
	}
	return nil
}

// Apply applies an overlay to a loaded config directory, mutating it in place.
func Apply(cfg ConfigDir, o *overlay.Overlay) error {
	for i, patch := range o.Patches {
		sel, err := selector.Parse(patch.Target)
		if err != nil {
			return fmt.Errorf("patch %d: %w", i, err)
		}

		if sel.IsArray() {
			if err := applyArrayPatch(cfg, sel, &patch); err != nil {
				return fmt.Errorf("patch %d (%s): %w", i, patch.Target, err)
			}
		} else {
			if err := applySingletonPatch(cfg, sel, &patch); err != nil {
				return fmt.Errorf("patch %d (%s): %w", i, patch.Target, err)
			}
		}
	}
	return nil
}

// applyArrayPatch handles patches targeting array sections (components, installs, actions).
// These live in subdirectories (e.g., components/foo.toml) as individual files.
func applyArrayPatch(cfg ConfigDir, sel *selector.Selector, patch *overlay.Patch) error {
	prefix := sel.Section + string(filepath.Separator)

	for relPath, doc := range cfg {
		if !strings.HasPrefix(relPath, prefix) {
			continue
		}

		if !sel.MatchesItem(doc) {
			continue
		}

		switch patch.Strategy {
		case overlay.StrategyMerge:
			deepMerge(doc, patch.Set)
		case overlay.StrategyReplace:
			cfg[relPath] = patch.Value
		case overlay.StrategyDelete:
			delete(cfg, relPath)
		}
	}

	return nil
}

// applySingletonPatch handles patches targeting singleton sections (sandbox, runner, policies, etc.).
// These are typically a single file like sandbox.toml, runner.toml, or a key in the root nuon.toml.
func applySingletonPatch(cfg ConfigDir, sel *selector.Selector, patch *overlay.Patch) error {
	// Try dedicated file first (e.g., "sandbox.toml", "policies.toml")
	filename := sel.Section + ".toml"
	doc, found := cfg[filename]

	// Also check root nuon.toml for inline sections
	rootDoc, hasRoot := cfg["nuon.toml"]

	switch patch.Strategy {
	case overlay.StrategyDelete:
		if found {
			delete(cfg, filename)
		}
		if hasRoot {
			delete(rootDoc, sel.Section)
		}
		return nil

	case overlay.StrategyReplace:
		if found {
			cfg[filename] = patch.Value
		} else if hasRoot {
			rootDoc[sel.Section] = patch.Value
		} else {
			// Create the file
			cfg[filename] = patch.Value
		}
		return nil

	case overlay.StrategyMerge:
		if found {
			deepMerge(doc, patch.Set)
		} else if hasRoot {
			section, ok := rootDoc[sel.Section]
			if ok {
				if sectionMap, ok := section.(map[string]any); ok {
					deepMerge(sectionMap, patch.Set)
				}
			} else {
				rootDoc[sel.Section] = patch.Set
			}
		} else {
			// Create the file with just the set fields
			cfg[filename] = copyMap(patch.Set)
		}
		return nil
	}

	return nil
}

// deepMerge merges src into dst recursively. Scalars in src overwrite dst.
// Maps are recursively merged. Everything else overwrites.
func deepMerge(dst, src map[string]any) {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		// If both are maps, recurse
		srcMap, srcOK := srcVal.(map[string]any)
		dstMap, dstOK := dstVal.(map[string]any)
		if srcOK && dstOK {
			deepMerge(dstMap, srcMap)
			continue
		}

		// Otherwise overwrite
		dst[key] = srcVal
	}
}

func copyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
