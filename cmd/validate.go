package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nuonco/nuon-ext-overlays/internal/overlay"
	"github.com/nuonco/nuon-ext-overlays/internal/patcher"
	"github.com/nuonco/nuon-ext-overlays/internal/selector"
)

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate overlay.toml syntax and selectors against the config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := patcher.LoadConfigDir(appDir)
			if err != nil {
				return fmt.Errorf("loading config dir: %w", err)
			}

			for _, overlayFile := range overlayFiles {
				o, err := overlay.ParseFile(resolveOverlayPath(overlayFile))
				if err != nil {
					return fmt.Errorf("overlay %s: %w", overlayFile, err)
				}

				if err := o.Validate(); err != nil {
					return fmt.Errorf("overlay %s: %w", overlayFile, err)
				}

				// Check that selectors actually match something
				for i, p := range o.Patches {
					sel, err := selector.Parse(p.Target)
					if err != nil {
						return fmt.Errorf("overlay %s patch %d: %w", overlayFile, i, err)
					}

					matches := countMatches(cfg, sel)
					if matches == 0 {
						fmt.Printf("⚠ overlay %s patch %d: selector %q matched 0 targets\n", overlayFile, i, p.Target)
					} else {
						fmt.Printf("✓ overlay %s patch %d: selector %q matched %d target(s)\n", overlayFile, i, p.Target, matches)
					}
				}
			}

			fmt.Println("\nValidation passed.")
			return nil
		},
	}
}

func countMatches(cfg patcher.ConfigDir, sel *selector.Selector) int {
	count := 0

	if sel.IsArray() {
		prefix := sel.Section + "/"
		for relPath, doc := range cfg {
			if len(relPath) <= len(prefix) {
				continue
			}
			if relPath[:len(prefix)] != prefix {
				continue
			}
			if sel.MatchesItem(doc) {
				count++
			}
		}
		return count
	}

	// Singleton: check for dedicated file or root key
	filename := sel.Section + ".toml"
	if _, ok := cfg[filename]; ok {
		return 1
	}
	if rootDoc, ok := cfg["nuon.toml"]; ok {
		if _, ok := rootDoc[sel.Section]; ok {
			return 1
		}
	}
	return 0
}
