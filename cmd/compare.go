package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nuonco/nuon-ext-overlays/internal/api"
	"github.com/nuonco/nuon-ext-overlays/internal/config"
	"github.com/nuonco/nuon-ext-overlays/internal/patcher"
)

func compareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare",
		Short: "Compare local app config against live config from the Nuon API",
		Long: `Loads the local directory (--dir flag or current directory) through
the TOML parser, fetches the live app config from the Nuon API, and compares
the normalized data structures.

Requires NUON_API_TOKEN, NUON_ORG_ID, and NUON_APP_ID environment variables
(set automatically when run via 'nuon overlays compare').

Example:
  nuon overlays compare
  nuon overlays compare -d ./app-aws`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			localDir := appDir

			local, err := patcher.LoadConfigBundle(localDir, nil)
			if err != nil {
				return fmt.Errorf("loading local dir %s: %w", localDir, err)
			}

			normalizeArrayKeys(local)
			inferDependencies(local)
			normalizeECRToPublic(local)
			inlineFileRefs(local, localDir)
			removeInlinedAssets(local)
			removeNonConfigFiles(local)

			cfg := config.Load()
			client, err := api.NewClient(cfg)
			if err != nil {
				return fmt.Errorf("setting up API client: %w", err)
			}

			live, err := client.FetchLiveConfig(context.Background())
			if err != nil {
				return err
			}

			result := compareBundles(local, live)
			printCompareResult(os.Stdout, result, localDir)

			if result.Mismatches > 0 || result.ExtraLeft > 0 || result.ExtraRight > 0 {
				os.Exit(1)
			}
			return nil
		},
	}
}

type compareResult struct {
	Lines      []compareLine
	Matches    int
	Mismatches int
	ExtraLeft  int
	ExtraRight int
}

type compareLine struct {
	File   string
	Status string // "match", "mismatch", "extra-left", "extra-right"
	Detail string // mismatch detail
	IsToml bool
}

func compareBundles(left, right *patcher.ConfigBundle) compareResult {
	var result compareResult

	// Normalize types so reflect.DeepEqual works across TOML/JSON sources.
	for _, doc := range left.Toml {
		normalizeTypes(doc)
	}
	for _, doc := range right.Toml {
		normalizeTypes(doc)
	}

	// Compare TOML files
	allToml := make(map[string]bool)
	for f := range left.Toml {
		allToml[f] = true
	}
	for f := range right.Toml {
		allToml[f] = true
	}

	sortedToml := sortedKeys(allToml)
	for _, file := range sortedToml {
		leftDoc, inLeft := left.Toml[file]
		rightDoc, inRight := right.Toml[file]

		switch {
		case inLeft && !inRight:
			result.Lines = append(result.Lines, compareLine{File: file, Status: "extra-left", IsToml: true})
			result.ExtraLeft++
		case !inLeft && inRight:
			result.Lines = append(result.Lines, compareLine{File: file, Status: "extra-right", IsToml: true})
			result.ExtraRight++
		default:
			if reflect.DeepEqual(leftDoc, rightDoc) {
				result.Lines = append(result.Lines, compareLine{File: file, Status: "match", IsToml: true})
				result.Matches++
			} else {
				detail := diffMaps("", leftDoc, rightDoc)
				result.Lines = append(result.Lines, compareLine{File: file, Status: "mismatch", Detail: detail, IsToml: true})
				result.Mismatches++
			}
		}
	}

	// Compare asset files
	allAssets := make(map[string]bool)
	for f := range left.Assets {
		allAssets[f] = true
	}
	for f := range right.Assets {
		allAssets[f] = true
	}

	sortedAssets := sortedKeys(allAssets)
	for _, file := range sortedAssets {
		leftAsset, inLeft := left.Assets[file]
		rightAsset, inRight := right.Assets[file]

		switch {
		case inLeft && !inRight:
			result.Lines = append(result.Lines, compareLine{File: file, Status: "extra-left"})
			result.ExtraLeft++
		case !inLeft && inRight:
			result.Lines = append(result.Lines, compareLine{File: file, Status: "extra-right"})
			result.ExtraRight++
		default:
			if assetsEqual(file, leftAsset.Bytes, rightAsset.Bytes) {
				result.Lines = append(result.Lines, compareLine{File: file, Status: "match"})
				result.Matches++
			} else {
				detail := fmt.Sprintf("byte content differs (%d vs %d bytes)", len(leftAsset.Bytes), len(rightAsset.Bytes))
				result.Lines = append(result.Lines, compareLine{File: file, Status: "mismatch", Detail: detail})
				result.Mismatches++
			}
		}
	}

	return result
}

func printCompareResult(w *os.File, r compareResult, localDir string) {
	fmt.Fprintf(w, "Comparing local (%s) vs live API config\n\n", localDir)

	for _, l := range r.Lines {
		kind := "asset"
		if l.IsToml {
			kind = "toml"
		}
		switch l.Status {
		case "match":
			fmt.Fprintf(w, "\033[32m✓\033[0m %s (%s)\n", l.File, kind)
		case "mismatch":
			fmt.Fprintf(w, "\033[31m✗\033[0m %s (%s)\n", l.File, kind)
			if l.Detail != "" {
				for _, line := range strings.Split(l.Detail, "\n") {
					if line != "" {
						fmt.Fprintf(w, "    %s\n", line)
					}
				}
			}
		case "extra-left":
			fmt.Fprintf(w, "\033[33m⚠\033[0m %s — only in local (%s)\n", l.File, kind)
		case "extra-right":
			fmt.Fprintf(w, "\033[33m⚠\033[0m %s — only in live (%s)\n", l.File, kind)
		}
	}

	total := r.Matches + r.Mismatches + r.ExtraLeft + r.ExtraRight
	fmt.Fprintf(w, "\n%d/%d files match", r.Matches, total)
	if r.Mismatches > 0 {
		fmt.Fprintf(w, ", %d mismatch", r.Mismatches)
	}
	if r.ExtraLeft > 0 {
		fmt.Fprintf(w, ", %d only in local", r.ExtraLeft)
	}
	if r.ExtraRight > 0 {
		fmt.Fprintf(w, ", %d only in live", r.ExtraRight)
	}
	fmt.Fprintln(w)
}

// diffMaps walks two maps recursively and returns a human-readable description
// of the first differences found.
func diffMaps(prefix string, left, right map[string]any) string {
	var diffs []string

	allKeys := make(map[string]bool)
	for k := range left {
		allKeys[k] = true
	}
	for k := range right {
		allKeys[k] = true
	}

	sorted := sortedKeys(allKeys)
	for _, k := range sorted {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		lv, inL := left[k]
		rv, inR := right[k]

		switch {
		case inL && !inR:
			diffs = append(diffs, fmt.Sprintf("%s: only in local = %v", path, lv))
		case !inL && inR:
			diffs = append(diffs, fmt.Sprintf("%s: only in live = %v", path, rv))
		default:
			lm, lOK := lv.(map[string]any)
			rm, rOK := rv.(map[string]any)
			if lOK && rOK {
				if sub := diffMaps(path, lm, rm); sub != "" {
					diffs = append(diffs, sub)
				}
			} else if !reflect.DeepEqual(lv, rv) {
				diffs = append(diffs, fmt.Sprintf("%s: local(%v) vs live(%v)", path, lv, rv))
			}
		}
	}

	return strings.Join(diffs, "\n")
}

// normalizeArrayKeys strips cloud-specific suffixes (.aws, .shared, .gcp, .azure)
// from component and action filenames so they match the API's naming convention.
func normalizeArrayKeys(bundle *patcher.ConfigBundle) {
	suffixes := []string{".aws", ".shared", ".gcp", ".azure"}
	prefixes := []string{"components/", "actions/"}

	normalized := make(patcher.ConfigDir, len(bundle.Toml))
	for key, doc := range bundle.Toml {
		newKey := key
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) {
				base := strings.TrimPrefix(key, prefix)
				base = strings.TrimSuffix(base, ".toml")
				for _, suffix := range suffixes {
					base = strings.TrimSuffix(base, suffix)
				}
				newKey = prefix + base + ".toml"
				break
			}
		}
		normalized[newKey] = doc
	}
	bundle.Toml = normalized
}

// inlineFileRefs resolves var_file/values_file entries whose "contents" is a
// relative file path (e.g. "./sandbox.tfvars") by reading the file and
// replacing the path with the actual file contents. This matches how the API
// stores these values (inlined).
func inlineFileRefs(bundle *patcher.ConfigBundle, baseDir string) {
	fileRefKeys := []string{"var_file", "values_file", "policy"}
	for tomlKey, doc := range bundle.Toml {
		for _, key := range fileRefKeys {
			arr, ok := doc[key]
			if !ok {
				continue
			}
			items := toAnySlice(arr)
			if items == nil {
				continue
			}
			// Build search paths: base dir, TOML file's parent dir,
			// and a sibling directory named after the TOML file
			// (e.g. policies.toml → policies/).
			tomlDir := filepath.Join(baseDir, filepath.Dir(tomlKey))
			sectionDir := filepath.Join(baseDir, strings.TrimSuffix(tomlKey, ".toml"))
			searchDirs := []string{tomlDir, sectionDir, baseDir}

			for _, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				contents, ok := m["contents"].(string)
				if !ok || contents == "" {
					continue
				}
				if !looksLikeFilePath(contents) {
					continue
				}
				for _, dir := range searchDirs {
					path := filepath.Join(dir, contents)
					data, err := os.ReadFile(path)
					if err == nil {
						m["contents"] = string(data)
						// Remove the inlined asset from the bundle so it
						// doesn't show up as "only in local".
						rel, _ := filepath.Rel(baseDir, path)
						if rel != "" {
							delete(bundle.Assets, rel)
						}
						break
					}
				}
			}
		}
	}
}

// nonConfigFiles are local-only files that don't correspond to anything in
// the app config API endpoint and should be excluded from comparison.
var nonConfigFiles = map[string]bool{
	"metadata.toml":  true, // app-level metadata (description, display_name) not in config endpoint
	"installer.toml": true, // separate installer/marketplace config
}

// removeNonConfigFiles removes local-only files that have no API equivalent.
func removeNonConfigFiles(bundle *patcher.ConfigBundle) {
	for key := range nonConfigFiles {
		delete(bundle.Toml, key)
	}
}

// removeInlinedAssets removes asset files from the bundle whose contents were
// inlined into a TOML document (e.g. policies/*.yml files referenced by
// policies.toml). These are redundant for comparison since the data is already
// compared via the TOML entry.
func removeInlinedAssets(bundle *patcher.ConfigBundle) {
	for tomlKey := range bundle.Toml {
		sectionDir := strings.TrimSuffix(tomlKey, ".toml") + "/"
		for assetKey := range bundle.Assets {
			if strings.HasPrefix(assetKey, sectionDir) {
				delete(bundle.Assets, assetKey)
			}
		}
	}
}

// looksLikeFilePath returns true if the string looks like a relative file path
// rather than inline content.
func looksLikeFilePath(s string) bool {
	if strings.HasPrefix(s, ".") {
		return true
	}
	exts := []string{".tfvars", ".hcl", ".yml", ".yaml", ".json", ".toml"}
	for _, ext := range exts {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

func toAnySlice(v any) []any {
	switch val := v.(type) {
	case []any:
		return val
	case []map[string]any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = item
		}
		return out
	}
	return nil
}

// normalizeTypes recursively converts typed slices (e.g. []map[string]any)
// to []any so reflect.DeepEqual works across TOML-parsed and JSON-parsed data.
// It also normalizes int64 to float64 (TOML uses int64, JSON uses float64)
// and removes empty maps/slices.
func normalizeTypes(m map[string]any) {
	for k, v := range m {
		nv := normalizeValue(v)
		if isEmpty(nv) {
			delete(m, k)
		} else {
			m[k] = nv
		}
	}
}

func isEmpty(v any) bool {
	switch val := v.(type) {
	case map[string]any:
		return len(val) == 0
	case []any:
		return len(val) == 0
	}
	return false
}

func normalizeValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		normalizeTypes(val)
		return val
	case []any:
		for i, item := range val {
			val[i] = normalizeValue(item)
		}
		return val
	case []map[string]any:
		out := make([]any, len(val))
		for i, item := range val {
			normalizeTypes(item)
			out[i] = item
		}
		return out
	case string:
		// Normalize JSON strings so key ordering and whitespace don't cause false mismatches.
		trimmed := strings.TrimSpace(val)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			var parsed any
			if json.Unmarshal([]byte(trimmed), &parsed) == nil {
				canonical, err := json.Marshal(parsed)
				if err == nil {
					return string(canonical)
				}
			}
		}
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	default:
		return v
	}
}

// normalizeECRToPublic converts local aws_ecr component config to the public
// format (just image_url + tag) to match what the API returns, since the API
// doesn't persist ECR-specific fields like region and iam_role_arn.
func normalizeECRToPublic(bundle *patcher.ConfigBundle) {
	for _, doc := range bundle.Toml {
		ecr, ok := doc["aws_ecr"].(map[string]any)
		if !ok {
			continue
		}
		pub := make(map[string]any)
		if v, ok := ecr["image_url"]; ok {
			pub["image_url"] = v
		}
		if v, ok := ecr["tag"]; ok {
			pub["tag"] = v
		}
		delete(doc, "aws_ecr")
		if len(pub) > 0 {
			doc["public"] = pub
		}
	}
}

// componentRefRe matches template references like {{ .nuon.components.NAME.outputs.* }}
var componentRefRe = regexp.MustCompile(`\{\{\s*\.nuon\.components\.([a-zA-Z0-9_]+)\.`)

// inferDependencies scans component TOML docs for template references to other
// components and populates the "dependencies" field to match what the API returns.
func inferDependencies(bundle *patcher.ConfigBundle) {
	for key, doc := range bundle.Toml {
		if !strings.HasPrefix(key, "components/") {
			continue
		}
		deps := extractComponentRefs(doc)
		if len(deps) > 0 {
			sorted := make([]any, len(deps))
			i := 0
			for d := range deps {
				sorted[i] = d
				i++
			}
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].(string) < sorted[j].(string) })
			doc["dependencies"] = sorted
		}
	}
}

// extractComponentRefs walks all string values in a map and returns the set
// of component names referenced via {{ .nuon.components.NAME.* }} templates.
func extractComponentRefs(m map[string]any) map[string]bool {
	refs := make(map[string]bool)
	walkStrings(m, func(s string) {
		for _, match := range componentRefRe.FindAllStringSubmatch(s, -1) {
			refs[match[1]] = true
		}
	})
	return refs
}

func walkStrings(v any, fn func(string)) {
	switch val := v.(type) {
	case string:
		fn(val)
	case map[string]any:
		for _, child := range val {
			walkStrings(child, fn)
		}
	case []any:
		for _, item := range val {
			walkStrings(item, fn)
		}
	case []map[string]any:
		for _, item := range val {
			walkStrings(item, fn)
		}
	}
}

// assetsEqual compares two asset byte slices. For .json files, it parses
// and compares semantically (ignoring key order and whitespace).
func assetsEqual(file string, a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	if strings.HasSuffix(file, ".json") {
		var va, vb any
		if json.Unmarshal(a, &va) == nil && json.Unmarshal(b, &vb) == nil {
			return reflect.DeepEqual(va, vb)
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
