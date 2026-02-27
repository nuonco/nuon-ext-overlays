package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nuonco/nuon-ext-overlays/internal/overlay"
)

var (
	overlayFiles []string
	appDir       string
)

func Execute() {
	root := &cobra.Command{
		Use:   "nuon-ext-overlays",
		Short: "Apply Kustomize-style config overlays to Nuon app configurations",
		Long: `Config overlays let you declaratively toggle platform features on/off
without editing individual component or install TOML files.

Define an overlay.toml with patches that target config sections:

  [[patches]]
  target = "components[*]"
  [patches.set]
  drift_schedule = ""

  [[patches]]
  target = 'installs[name="dev"]'
  [patches.set]
  approval_option = "approve-all"

  [[patches]]
  target = "policies"
  strategy = "delete"

Commands:
  preview   Show what the overlay would change (no sync)
  apply     Apply overlay and write patched config
  validate  Check overlay.toml syntax and selectors
  init      Generate a starter overlay.toml from existing config`,
	}

	root.PersistentFlags().StringArrayVarP(&overlayFiles, "overlay", "f", []string{"overlay.toml"}, "overlay file(s) to apply (repeatable, applied left-to-right)")
	root.PersistentFlags().StringVarP(&appDir, "dir", "d", ".", "path to the Nuon app config directory")

	// The extension runner (script type) sets cmd.Dir to the extension
	// directory, so relative paths resolve against the wrong CWD.
	// PWD in the inherited environment still holds the caller's original
	// working directory, so we use that to resolve relative paths.
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if !filepath.IsAbs(appDir) {
			if pwd := os.Getenv("PWD"); pwd != "" {
				appDir = filepath.Join(pwd, appDir)
			} else {
				abs, err := filepath.Abs(appDir)
				if err != nil {
					return fmt.Errorf("resolving app dir: %w", err)
				}
				appDir = abs
			}
		}
		return nil
	}

	root.AddCommand(
		previewCmd(),
		applyCmd(),
		validateCmd(),
		initCmd(),
		compareCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// resolveOverlayPath returns the overlay file path, trying the literal path
// first, then falling back to resolving it relative to the app config dir.
func resolveOverlayPath(overlayFile string) string {
	if filepath.IsAbs(overlayFile) {
		return overlayFile
	}
	// Try resolving relative to the caller's original PWD first,
	// since the extension runner may have changed the working directory.
	if pwd := os.Getenv("PWD"); pwd != "" {
		candidate := filepath.Join(pwd, overlayFile)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if _, err := os.Stat(overlayFile); err == nil {
		return overlayFile
	}
	return filepath.Join(appDir, overlayFile)
}

// collectBases parses each overlay file and returns deduplicated, resolved
// base directory paths (order preserved). Each base path in an overlay is
// resolved relative to the overlay file's directory.
func collectBases(overlayFileArgs []string) ([]string, error) {
	seen := make(map[string]bool)
	var bases []string

	for _, overlayFile := range overlayFileArgs {
		resolved := resolveOverlayPath(overlayFile)
		o, err := overlay.ParseFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("parsing %s for bases: %w", overlayFile, err)
		}

		overlayDir := filepath.Dir(resolved)
		for _, b := range o.Bases {
			abs := filepath.Clean(filepath.Join(overlayDir, b))
			if !seen[abs] {
				seen[abs] = true
				bases = append(bases, abs)
			}
		}
	}

	return bases, nil
}
