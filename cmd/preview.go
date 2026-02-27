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

func previewCmd() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Show what the overlay would change without applying",
		Long: `Shows a colored diff of what the overlay would change.

Optionally write the patched config to a directory with --output:
  nuon overlays preview -d ./my-app -o /tmp/patched`,
		RunE: func(cmd *cobra.Command, args []string) error {
			original, err := patcher.LoadConfigDir(appDir)
			if err != nil {
				return fmt.Errorf("loading config dir: %w", err)
			}

			patched, err := patcher.LoadConfigDir(appDir)
			if err != nil {
				return fmt.Errorf("loading config dir: %w", err)
			}

			for _, overlayFile := range overlayFiles {
				o, err := overlay.ParseFile(resolveOverlayPath(overlayFile))
				if err != nil {
					return err
				}
				if err := o.Validate(); err != nil {
					return fmt.Errorf("overlay %s: %w", overlayFile, err)
				}
				if err := patcher.Apply(patched, o); err != nil {
					return fmt.Errorf("applying %s: %w", overlayFile, err)
				}
			}

			diffs := preview.Generate(original, patched)
			preview.PrintDiffs(os.Stdout, diffs)

			if len(diffs) == 0 {
				return nil
			}

			// Write patched config if output dir specified
			if outputDir != "" {
				if !filepath.IsAbs(outputDir) {
					if pwd := os.Getenv("PWD"); pwd != "" {
						outputDir = filepath.Join(pwd, outputDir)
					}
				}
				if err := patcher.WriteConfigDir(patched, outputDir); err != nil {
					return fmt.Errorf("writing patched config: %w", err)
				}
				fmt.Printf("\nPatched config written to: %s\n", outputDir)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "write patched config to this directory")

	return cmd
}
