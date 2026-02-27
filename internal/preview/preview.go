package preview

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nuonco/nuon-ext-overlays/internal/patcher"
)

// Diff represents the changes between original and patched config.
type Diff struct {
	File    string
	Status  string // "modified", "deleted", "added"
	Before  string
	After   string
}

// Generate computes diffs between original and patched config directories.
func Generate(original, patched patcher.ConfigDir) []Diff {
	var diffs []Diff

	allFiles := make(map[string]bool)
	for f := range original {
		allFiles[f] = true
	}
	for f := range patched {
		allFiles[f] = true
	}

	sorted := make([]string, 0, len(allFiles))
	for f := range allFiles {
		sorted = append(sorted, f)
	}
	sort.Strings(sorted)

	for _, file := range sorted {
		origDoc, hadOrig := original[file]
		patchDoc, hadPatch := patched[file]

		if hadOrig && !hadPatch {
			diffs = append(diffs, Diff{
				File:   file,
				Status: "deleted",
				Before: encode(origDoc),
			})
			continue
		}

		if !hadOrig && hadPatch {
			diffs = append(diffs, Diff{
				File:   file,
				Status: "added",
				After:  encode(patchDoc),
			})
			continue
		}

		before := encode(origDoc)
		after := encode(patchDoc)
		if before != after {
			diffs = append(diffs, Diff{
				File:   file,
				Status: "modified",
				Before: before,
				After:  after,
			})
		}
	}

	return diffs
}

// PrintDiffs writes a human-readable diff to the writer.
func PrintDiffs(w io.Writer, diffs []Diff) {
	if len(diffs) == 0 {
		fmt.Fprintln(w, "No changes.")
		return
	}

	for _, d := range diffs {
		switch d.Status {
		case "deleted":
			fmt.Fprintf(w, "\033[31m--- %s (deleted)\033[0m\n", d.File)
			for _, line := range strings.Split(d.Before, "\n") {
				if line != "" {
					fmt.Fprintf(w, "\033[31m- %s\033[0m\n", line)
				}
			}
		case "added":
			fmt.Fprintf(w, "\033[32m+++ %s (added)\033[0m\n", d.File)
			for _, line := range strings.Split(d.After, "\n") {
				if line != "" {
					fmt.Fprintf(w, "\033[32m+ %s\033[0m\n", line)
				}
			}
		case "modified":
			fmt.Fprintf(w, "\033[33m~~~ %s (modified)\033[0m\n", d.File)
			printLineDiff(w, d.Before, d.After)
		}
		fmt.Fprintln(w)
	}
}

func printLineDiff(w io.Writer, before, after string) {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	beforeSet := make(map[string]bool)
	for _, l := range beforeLines {
		if l != "" {
			beforeSet[l] = true
		}
	}
	afterSet := make(map[string]bool)
	for _, l := range afterLines {
		if l != "" {
			afterSet[l] = true
		}
	}

	for _, l := range beforeLines {
		if l == "" {
			continue
		}
		if !afterSet[l] {
			fmt.Fprintf(w, "\033[31m- %s\033[0m\n", l)
		}
	}
	for _, l := range afterLines {
		if l == "" {
			continue
		}
		if !beforeSet[l] {
			fmt.Fprintf(w, "\033[32m+ %s\033[0m\n", l)
		}
	}
}

func encode(doc map[string]any) string {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Encode(doc)
	return buf.String()
}
