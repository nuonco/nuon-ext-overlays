package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nuonco/nuon-ext-overlays/internal/overlay"
	"github.com/nuonco/nuon-ext-overlays/internal/patcher"
	"github.com/nuonco/nuon-ext-overlays/internal/preview"
)

func applyCmd() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply overlay and write patched config to an output directory",
		Long: `Reads the app config directory, applies all overlay patches, and writes
the result to the output directory. The original config is never modified.

Use --output to specify the destination (defaults to a temp directory).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			bases, err := collectBases(overlayFiles)
			if err != nil {
				return err
			}

			original, err := patcher.LoadConfigBundle(appDir, bases)
			if err != nil {
				return fmt.Errorf("loading config bundle: %w", err)
			}

			patched, err := patcher.LoadConfigBundle(appDir, bases)
			if err != nil {
				return fmt.Errorf("loading config bundle: %w", err)
			}

			for _, overlayFile := range overlayFiles {
				o, err := overlay.ParseFile(resolveOverlayPath(overlayFile))
				if err != nil {
					return err
				}
				if err := o.Validate(); err != nil {
					return fmt.Errorf("overlay %s: %w", overlayFile, err)
				}
				if err := patcher.Apply(patched.Toml, o); err != nil {
					return fmt.Errorf("applying %s: %w", overlayFile, err)
				}
			}

			// Show diff
			diffs := preview.Generate(original.Toml, patched.Toml)
			if len(diffs) == 0 {
				fmt.Println("No patch changes.")
			} else {
				preview.PrintDiffs(os.Stdout, diffs)
			}

			// Determine output directory
			destDir := outputDir
			if destDir == "" {
				tmp, err := os.MkdirTemp("", "nuon-overlay-*")
				if err != nil {
					return fmt.Errorf("creating temp dir: %w", err)
				}
				destDir = tmp
			}

			// Write patched config
			if err := patcher.WriteConfigBundle(patched, destDir); err != nil {
				return fmt.Errorf("writing patched config: %w", err)
			}

			absDir, _ := filepath.Abs(destDir)
			fmt.Printf("\nPatched config written to: %s\n", absDir)
			fmt.Println("\nTo sync this config, run:")
			fmt.Printf("  nuon sync --dir %s\n", absDir)

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "output directory for patched config (default: temp dir)")

	return cmd
}
