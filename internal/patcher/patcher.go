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

// Asset represents a non-TOML file in a config directory.
type Asset struct {
	Bytes []byte
	Mode  os.FileMode
}

// ConfigBundle holds both parsed TOML files and raw asset files from a config directory.
type ConfigBundle struct {
	Toml   ConfigDir
	Assets map[string]Asset // non-.toml files keyed by relative path
}

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

// skipDir returns true for directories that should be ignored during config loading.
func skipDir(name string) bool {
	switch name {
	case ".git", ".jj", ".github":
		return true
	}
	return false
}

// skipFile returns true for files that should be ignored during config loading.
func skipFile(name string) bool {
	return name == "overlay.toml" || strings.HasPrefix(name, "overlay") && strings.HasSuffix(name, ".toml")
}

// arraySections are TOML section names that hold arrays of items with unique "name" fields.
var arraySections = []string{"components", "actions", "installs"}

// LoadConfigBundle loads a composed config bundle from a main directory and optional base directories.
// Sources are applied in order: baseDirs first (in order), then mainDir (highest precedence).
func LoadConfigBundle(mainDir string, baseDirs []string) (*ConfigBundle, error) {
	bundle := &ConfigBundle{
		Toml:   make(ConfigDir),
		Assets: make(map[string]Asset),
	}

	// Build source list: bases first, main last (highest precedence)
	sources := make([]string, 0, len(baseDirs)+1)
	sources = append(sources, baseDirs...)
	sources = append(sources, mainDir)

	for _, srcDir := range sources {
		err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				if skipDir(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			rel, err := filepath.Rel(srcDir, path)
			if err != nil {
				return err
			}

			if skipFile(info.Name()) {
				return nil
			}

			if strings.HasSuffix(info.Name(), ".toml") {
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("reading %s: %w", rel, err)
				}
				var doc map[string]any
				if err := toml.Unmarshal(data, &doc); err != nil {
					return fmt.Errorf("parsing %s: %w", rel, err)
				}
				bundle.Toml[rel] = doc
			} else {
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("reading %s: %w", rel, err)
				}
				if len(data) == 0 {
					return nil
				}
				bundle.Assets[rel] = Asset{
					Bytes: data,
					Mode:  info.Mode(),
				}
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", srcDir, err)
		}
	}

	// Validate no duplicate names in array sections across different files.
	for _, section := range arraySections {
		prefix := section + string(filepath.Separator)
		seen := make(map[string]string) // name -> source file
		for relPath, doc := range bundle.Toml {
			if !strings.HasPrefix(relPath, prefix) {
				continue
			}
			nameVal, ok := doc["name"]
			if !ok {
				continue
			}
			name, ok := nameVal.(string)
			if !ok {
				continue
			}
			if prev, dup := seen[name]; dup {
				return nil, fmt.Errorf("duplicate %s name %q in %s and %s", section, name, prev, relPath)
			}
			seen[name] = relPath
		}
	}

	return bundle, nil
}

// WriteConfigBundle writes both TOML files and raw asset files to a destination directory.
func WriteConfigBundle(b *ConfigBundle, destDir string) error {
	if err := WriteConfigDir(b.Toml, destDir); err != nil {
		return err
	}

	for relPath, asset := range b.Assets {
		fullPath := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", relPath, err)
		}
		if err := os.WriteFile(fullPath, asset.Bytes, asset.Mode); err != nil {
			return fmt.Errorf("writing %s: %w", relPath, err)
		}
	}

	return nil
}
