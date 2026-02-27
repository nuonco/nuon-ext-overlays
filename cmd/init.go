package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nuonco/nuon-ext-overlays/internal/patcher"
)

func initCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a starter overlay.toml from an existing config directory",
		Long: `Scans the app config directory and generates a commented overlay.toml
with example patches for each discoverable section (components, installs,
sandbox, policies, etc.).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			bases, err := collectBases(overlayFiles)
			if err != nil {
				// No existing overlay files is fine for init; use empty bases.
				bases = nil
			}

			bundle, err := patcher.LoadConfigBundle(appDir, bases)
			if err != nil {
				return fmt.Errorf("loading config bundle: %w", err)
			}

			content := generateOverlayTemplate(bundle.Toml)

			if output == "" || output == "-" {
				fmt.Print(content)
				return nil
			}

			if err := os.WriteFile(output, []byte(content), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", output, err)
			}
			fmt.Printf("Generated overlay template: %s\n", output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "overlay.toml", "output file path (- for stdout)")

	return cmd
}

func generateOverlayTemplate(cfg patcher.ConfigDir) string {
	var b strings.Builder

	b.WriteString("version = \"1\"\n\n")

	// Discover components
	var componentNames []string
	for relPath, doc := range cfg {
		if strings.HasPrefix(relPath, "components"+string(filepath.Separator)) {
			if name, ok := doc["name"]; ok {
				componentNames = append(componentNames, fmt.Sprintf("%v", name))
			}
		}
	}
	sort.Strings(componentNames)

	if len(componentNames) > 0 {
		b.WriteString("# ── Components ─────────────────────────────────────────────\n")
		b.WriteString("# Disable drift detection on all components:\n")
		b.WriteString("#\n")
		b.WriteString("# [[patches]]\n")
		b.WriteString("# target = \"components[*]\"\n")
		b.WriteString("# [patches.set]\n")
		b.WriteString("# drift_schedule = \"\"\n")
		b.WriteString("#\n")

		for _, name := range componentNames {
			b.WriteString(fmt.Sprintf("# Target specific component %q:\n", name))
			b.WriteString(fmt.Sprintf("# [[patches]]\n"))
			b.WriteString(fmt.Sprintf("# target = 'components[name=\"%s\"]'\n", name))
			b.WriteString("# [patches.set]\n")
			b.WriteString("# drift_schedule = \"\"\n")
			b.WriteString("#\n")
		}
		b.WriteString("\n")
	}

	// Discover installs
	var installNames []string
	for relPath, doc := range cfg {
		if strings.HasPrefix(relPath, "installs"+string(filepath.Separator)) {
			if name, ok := doc["name"]; ok {
				installNames = append(installNames, fmt.Sprintf("%v", name))
			}
		}
	}
	sort.Strings(installNames)

	if len(installNames) > 0 {
		b.WriteString("# ── Installs ───────────────────────────────────────────────\n")
		b.WriteString("# Auto-approve all installs:\n")
		b.WriteString("#\n")
		b.WriteString("# [[patches]]\n")
		b.WriteString("# target = \"installs[*]\"\n")
		b.WriteString("# [patches.set]\n")
		b.WriteString("# approval_option = \"approve-all\"\n")
		b.WriteString("#\n")

		for _, name := range installNames {
			b.WriteString(fmt.Sprintf("# Target install %q:\n", name))
			b.WriteString("# [[patches]]\n")
			b.WriteString(fmt.Sprintf("# target = 'installs[name=\"%s\"]'\n", name))
			b.WriteString("# [patches.set]\n")
			b.WriteString("# approval_option = \"approve-all\"\n")
			b.WriteString("#\n")
		}
		b.WriteString("\n")
	}

	// Singleton sections
	singletons := []struct {
		name    string
		example string
	}{
		{"sandbox", "drift_schedule = \"\""},
		{"policies", "# strategy = \"delete\"  # remove all policies"},
		{"runner", "# Add runner overrides here"},
		{"inputs", "# Add input overrides here"},
	}

	for _, s := range singletons {
		if hasSection(cfg, s.name) {
			b.WriteString(fmt.Sprintf("# ── %s ─────────────────────────────────────────────\n", strings.Title(s.name)))
			b.WriteString("#\n")
			b.WriteString("# [[patches]]\n")
			b.WriteString(fmt.Sprintf("# target = \"%s\"\n", s.name))
			b.WriteString("# [patches.set]\n")
			b.WriteString(fmt.Sprintf("# %s\n", s.example))
			b.WriteString("#\n\n")
		}
	}

	return b.String()
}

func hasSection(cfg patcher.ConfigDir, section string) bool {
	filename := section + ".toml"
	if _, ok := cfg[filename]; ok {
		return true
	}
	if rootDoc, ok := cfg["nuon.toml"]; ok {
		if _, ok := rootDoc[section]; ok {
			return true
		}
	}
	return false
}
