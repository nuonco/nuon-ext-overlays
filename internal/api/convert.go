package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nuonco/nuon/sdks/nuon-go/models"

	"github.com/nuonco/nuon-ext-overlays/internal/patcher"
)

// metadataKeys are API-only fields that don't exist in local TOML config.
var metadataKeys = map[string]bool{
	"id":                            true,
	"app_id":                        true,
	"app_config_id":                 true,
	"org_id":                        true,
	"created_at":                    true,
	"updated_at":                    true,
	"created_by_id":                 true,
	"checksum":                      true,
	"status":                        true,
	"status_description":            true,
	"state":                         true,
	"version":                       true, // app config version, not terraform version
	"cli_version":                   true,
	"app_branch_id":                 true,
	"app_branch":                    true,
	"vcs_connection_commit":         true,
	"component_ids":                 true,
	"component_id":                  true,
	"component_config_connection_id": true,
	"component_config_id":           true,
	"component_config_type":         true,
	"app_config_version":            true,
	"action_workflow_id":            true,
	"app_input_id":                  true,
	"group_id":                      true,
	"group":                         true,
	"index":                         true,
	"internal":                      true,
	"source":                        true,
	"vcs_connection":                true,
	"vcs_connection_id":             true,
	"repo_name":                     true,
	"repo_owner":                    true,
	"cloud_platform":                true,
	"readme":                        true,
	"cloudformation_stack_name":     true,
	"cloudformation_stack_parameter_name": true,
	"helm_config_json":              true,
	"action_workflow_config_id":      true,
	"previous_step_id":               true,
	"idx":                            true,
	"component":                      true,
	"aws_ecr_image_config":           true,
}

// toMap converts any struct to map[string]any via JSON roundtrip,
// then strips metadata keys.
func toMap(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	stripMetadata(m)
	return m
}

// stripMetadata removes API-only metadata keys recursively.
func stripMetadata(m map[string]any) {
	for k := range m {
		if metadataKeys[k] {
			delete(m, k)
			continue
		}
		if sub, ok := m[k].(map[string]any); ok {
			stripMetadata(sub)
			if len(sub) == 0 {
				delete(m, k)
			}
		}
		if arr, ok := m[k].([]any); ok {
			for _, item := range arr {
				if sub, ok := item.(map[string]any); ok {
					stripMetadata(sub)
				}
			}
		}
	}
}

// removeEmpty removes nil values and empty strings/slices/maps from a map.
func removeEmpty(m map[string]any) {
	for k, v := range m {
		if v == nil {
			delete(m, k)
			continue
		}
		switch val := v.(type) {
		case string:
			if val == "" {
				delete(m, k)
			}
		case []any:
			if len(val) == 0 {
				delete(m, k)
			}
		case map[string]any:
			removeEmpty(val)
			if len(val) == 0 {
				delete(m, k)
			}
		case float64:
			if val == 0 {
				delete(m, k)
			}
		case bool:
			if !val {
				delete(m, k)
			}
		}
	}
}

// convertVCS extracts public_repo or connected_repo from an API VCS config
// and writes it into the target map with TOML-style keys.
func convertVCS(target map[string]any, pub *models.AppPublicGitVCSConfig, conn *models.AppConnectedGithubVCSConfig) {
	if pub != nil {
		repo := make(map[string]any)
		if pub.Repo != "" {
			repo["repo"] = pub.Repo
		}
		if pub.Directory != "" {
			repo["directory"] = pub.Directory
		}
		if pub.Branch != "" {
			repo["branch"] = pub.Branch
		}
		if len(repo) > 0 {
			target["public_repo"] = repo
		}
	}
	if conn != nil {
		repo := make(map[string]any)
		if conn.Repo != "" {
			repo["repo"] = conn.Repo
		}
		if conn.Directory != "" {
			repo["directory"] = conn.Directory
		}
		if conn.Branch != "" {
			repo["branch"] = conn.Branch
		}
		if len(repo) > 0 {
			target["connected_repo"] = repo
		}
	}
}

// convertVariablesFiles converts API variables_files ([]string) to TOML var_file format.
func convertVariablesFiles(files []string) []any {
	if len(files) == 0 {
		return nil
	}
	var result []any
	for _, f := range files {
		result = append(result, map[string]any{"contents": f})
	}
	return result
}

// convertValuesFiles converts API values_files ([]string) to TOML values_file format.
func convertValuesFiles(files []string) []any {
	if len(files) == 0 {
		return nil
	}
	var result []any
	for _, f := range files {
		result = append(result, map[string]any{"contents": f})
	}
	return result
}

func convertSandbox(s *models.AppAppSandboxConfig) map[string]any {
	doc := make(map[string]any)
	if s.TerraformVersion != "" {
		doc["terraform_version"] = s.TerraformVersion
	}
	if s.DriftSchedule != "" {
		doc["drift_schedule"] = s.DriftSchedule
	}
	if len(s.Variables) > 0 {
		// API "variables" → TOML "vars"
		vars := make(map[string]any, len(s.Variables))
		for k, v := range s.Variables {
			vars[k] = v
		}
		doc["vars"] = vars
	}
	if len(s.EnvVars) > 0 {
		envVars := make(map[string]any, len(s.EnvVars))
		for k, v := range s.EnvVars {
			envVars[k] = v
		}
		doc["env_vars"] = envVars
	}
	if files := convertVariablesFiles(s.VariablesFiles); files != nil {
		doc["var_file"] = files
	}
	convertVCS(doc, s.PublicGitVcsConfig, s.ConnectedGithubVcsConfig)
	return doc
}

func convertRunner(r *models.AppAppRunnerConfig) map[string]any {
	doc := make(map[string]any)
	if string(r.AppRunnerType) != "" {
		doc["runner_type"] = string(r.AppRunnerType)
	}
	if r.HelmDriver != "" {
		doc["helm_driver"] = r.HelmDriver
	}
	if len(r.EnvVars) > 0 {
		envVars := make(map[string]any, len(r.EnvVars))
		for k, v := range r.EnvVars {
			envVars[k] = v
		}
		doc["env_vars"] = envVars
	}
	if r.InitScript != "" {
		doc["init_script_url"] = r.InitScript
	}
	return doc
}

func convertInput(input *models.AppAppInputConfig) map[string]any {
	doc := make(map[string]any)

	// Build group ID → name mapping
	groupNames := make(map[string]string)
	if len(input.InputGroups) > 0 {
		var groups []any
		for _, g := range input.InputGroups {
			groupNames[g.ID] = g.Name
			m := make(map[string]any)
			if g.Name != "" {
				m["name"] = g.Name
			}
			if g.Description != "" {
				m["description"] = g.Description
			}
			if g.DisplayName != "" {
				m["display_name"] = g.DisplayName
			}
			if len(m) > 0 {
				groups = append(groups, m)
			}
		}
		if len(groups) > 0 {
			doc["group"] = groups
		}
	}

	if len(input.Inputs) > 0 {
		var inputs []any
		for _, inp := range input.Inputs {
			m := make(map[string]any)
			if inp.Name != "" {
				m["name"] = inp.Name
			}
			if inp.Description != "" {
				m["description"] = inp.Description
			}
			if inp.Default != "" {
				m["default"] = inp.Default
			}
			if inp.Sensitive {
				m["sensitive"] = true
			}
			if inp.Required {
				m["required"] = true
			}
			if inp.DisplayName != "" {
				m["display_name"] = inp.DisplayName
			}
			if inp.Type != "" && inp.Type != "string" {
				m["type"] = inp.Type
			}
			// Resolve group_id to group name
			if inp.GroupID != "" {
				if name, ok := groupNames[inp.GroupID]; ok {
					m["group"] = name
				}
			}
			if len(m) > 0 {
				inputs = append(inputs, m)
			}
		}
		if len(inputs) > 0 {
			doc["input"] = inputs
		}
	}
	return doc
}

// convertPoliciesFromJSON extracts policies from the raw API JSON response.
// The SDK model (AppAppPoliciesConfig) doesn't capture the nested policies
// array, so we parse directly from JSON to build TOML-compatible [[policy]] entries.
func convertPoliciesFromJSON(rawJSON []byte) map[string]any {
	if len(rawJSON) == 0 {
		return nil
	}
	var envelope rawPoliciesEnvelope
	if err := json.Unmarshal(rawJSON, &envelope); err != nil {
		return nil
	}
	if envelope.Policies == nil || len(envelope.Policies.Policies) == 0 {
		return nil
	}

	doc := make(map[string]any)
	var policies []any
	for _, p := range envelope.Policies.Policies {
		m := make(map[string]any)
		if p.Type != "" {
			m["type"] = p.Type
		}
		if p.Contents != "" {
			m["contents"] = p.Contents
		}
		if len(m) > 0 {
			policies = append(policies, m)
		}
	}
	if len(policies) > 0 {
		doc["policy"] = policies
	}
	return doc
}

// permissionShortType maps API role types to the short type used in local TOML files.
func permissionShortType(apiType string) string {
	switch apiType {
	case "runner_provision":
		return "provision"
	case "runner_deprovision":
		return "deprovision"
	case "runner_maintenance":
		return "maintenance"
	default:
		return apiType
	}
}

// convertPermissions converts an API permissions config into per-role TOML documents
// and decoded asset files, matching the local permissions/ subdirectory structure.
func convertPermissions(p *models.AppAppPermissionsConfig, bundle *patcher.ConfigBundle) {
	if len(p.AwsIamRoles) == 0 {
		return
	}

	// Filter out breakglass roles — those are handled by convertBreakGlass.
	var roles []*models.AppAppAWSIAMRoleConfig
	for _, role := range p.AwsIamRoles {
		if string(role.Type) != "breakglass" {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return
	}

	typeCounts := make(map[string]int)
	for _, role := range roles {
		shortType := permissionShortType(string(role.Type))
		typeCounts[shortType]++
	}

	typeSeen := make(map[string]int)
	for _, role := range roles {
		shortType := permissionShortType(string(role.Type))
		typeSeen[shortType]++

		filename := shortType
		if typeCounts[shortType] > 1 {
			filename = fmt.Sprintf("%s_%d", shortType, typeSeen[shortType])
		}

		doc := make(map[string]any)
		doc["type"] = shortType
		if role.Name != "" {
			doc["name"] = role.Name
		}
		if role.Description != "" {
			doc["description"] = role.Description
		}
		if role.DisplayName != "" {
			doc["display_name"] = role.DisplayName
		}

		if role.PermissionsBoundary != "" {
			decoded, err := base64.StdEncoding.DecodeString(role.PermissionsBoundary)
			if err == nil {
				boundaryFile := filename + "_boundary.json"
				doc["permissions_boundary"] = "./" + boundaryFile
				bundle.Assets["permissions/"+boundaryFile] = patcher.Asset{Bytes: prettyJSON(decoded)}
			}
		}

		if len(role.Policies) > 0 {
			var policies []any
			for _, pol := range role.Policies {
				m := make(map[string]any)
				if pol.ManagedPolicyName != "" {
					m["managed_policy_name"] = pol.ManagedPolicyName
				}
				if pol.Name != "" {
					m["name"] = pol.Name
				}
				if pol.Contents != "" {
					decoded, err := base64.StdEncoding.DecodeString(pol.Contents)
					if err == nil {
						m["contents"] = string(prettyJSON(decoded))
					}
				}
				if len(m) > 0 {
					policies = append(policies, m)
				}
			}
			if len(policies) > 0 {
				doc["policies"] = policies
			}
		}

		bundle.Toml["permissions/"+filename+".toml"] = doc
	}
}

func convertBreakGlass(bg *models.AppAppBreakGlassConfig) map[string]any {
	if len(bg.AwsIamRoles) == 0 {
		return nil
	}
	doc := make(map[string]any)
	var roles []any
	for _, role := range bg.AwsIamRoles {
		m := make(map[string]any)
		if role.Name != "" {
			m["name"] = role.Name
		}
		if role.Description != "" {
			m["description"] = role.Description
		}
		if role.DisplayName != "" {
			m["display_name"] = role.DisplayName
		}
		if role.PermissionsBoundary != "" {
			m["permissions_boundary"] = role.PermissionsBoundary
		} else {
			m["permissions_boundary"] = ""
		}
		if len(role.Policies) > 0 {
			var policies []any
			for _, pol := range role.Policies {
				pm := make(map[string]any)
				if pol.ManagedPolicyName != "" {
					pm["managed_policy_name"] = pol.ManagedPolicyName
				}
				if pol.Name != "" {
					pm["name"] = pol.Name
				}
				if pol.Contents != "" {
					decoded, err := base64.StdEncoding.DecodeString(pol.Contents)
					if err == nil {
						pm["contents"] = string(prettyJSON(decoded))
					}
				}
				if len(pm) > 0 {
					policies = append(policies, pm)
				}
			}
			if len(policies) > 0 {
				m["policies"] = policies
			}
		}
		roles = append(roles, m)
	}
	doc["role"] = roles
	return doc
}

func convertStack(s *models.AppAppStackConfig) map[string]any {
	doc := make(map[string]any)
	if string(s.Type) != "" {
		doc["type"] = string(s.Type)
	}
	if s.Name != "" {
		doc["name"] = s.Name
	}
	if s.Description != "" {
		doc["description"] = s.Description
	}
	if s.VpcNestedTemplateURL != "" {
		doc["vpc_nested_template_url"] = s.VpcNestedTemplateURL
	}
	if s.RunnerNestedTemplateURL != "" {
		doc["runner_nested_template_url"] = s.RunnerNestedTemplateURL
	}
	return doc
}

func convertSecrets(s *models.AppAppSecretsConfig) map[string]any {
	doc := make(map[string]any)
	if len(s.Secrets) > 0 {
		var secrets []any
		for _, sec := range s.Secrets {
			m := make(map[string]any)
			if sec.Name != "" {
				m["name"] = sec.Name
			}
			if sec.DisplayName != "" {
				m["display_name"] = sec.DisplayName
			}
			if sec.Description != "" {
				m["description"] = sec.Description
			}
			if sec.Required {
				m["required"] = true
			}
			if sec.Default != "" {
				m["default"] = sec.Default
			}
			if sec.Format != "" {
				m["format"] = sec.Format
			}
			if len(m) > 0 {
				secrets = append(secrets, m)
			}
		}
		if len(secrets) > 0 {
			doc["secret"] = secrets
		}
	}
	return doc
}

func convertComponent(c *models.AppComponentConfigConnection, compNames map[string]string) map[string]any {
	doc := make(map[string]any)

	if c.ComponentName != "" {
		doc["name"] = c.ComponentName
	}
	if string(c.Type) != "" {
		t := string(c.Type)
		if t == "external_image" {
			t = "container_image"
		}
		doc["type"] = t
	}
	if c.DriftSchedule != "" {
		doc["drift_schedule"] = c.DriftSchedule
	}
	if len(c.ComponentDependencyIds) > 0 {
		deps := make([]any, len(c.ComponentDependencyIds))
		for i, id := range c.ComponentDependencyIds {
			if name, ok := compNames[id]; ok {
				deps[i] = name
			} else {
				deps[i] = id
			}
		}
		doc["dependencies"] = deps
	}

	switch {
	case c.TerraformModule != nil:
		convertTerraformModule(doc, c.TerraformModule)
	case c.Helm != nil:
		convertHelm(doc, c.Helm)
	case c.DockerBuild != nil:
		convertDockerBuild(doc, c.DockerBuild)
	case c.ExternalImage != nil:
		convertExternalImage(doc, c.ExternalImage)
	case c.Job != nil:
		convertJob(doc, c.Job)
	case c.KubernetesManifest != nil:
		convertKubernetesManifest(doc, c.KubernetesManifest)
	}

	removeEmpty(doc)
	return doc
}

func convertTerraformModule(doc map[string]any, tf *models.AppTerraformModuleComponentConfig) {
	// API "version" → TOML "terraform_version"
	if tf.Version != "" {
		doc["terraform_version"] = tf.Version
	}
	// API "variables" → TOML "vars"
	if len(tf.Variables) > 0 {
		vars := make(map[string]any, len(tf.Variables))
		for k, v := range tf.Variables {
			vars[k] = v
		}
		doc["vars"] = vars
	}
	if len(tf.EnvVars) > 0 {
		envVars := make(map[string]any, len(tf.EnvVars))
		for k, v := range tf.EnvVars {
			envVars[k] = v
		}
		doc["env_vars"] = envVars
	}
	if files := convertVariablesFiles(tf.VariablesFiles); files != nil {
		doc["var_file"] = files
	}
	convertVCS(doc, tf.PublicGitVcsConfig, tf.ConnectedGithubVcsConfig)
}

func convertHelm(doc map[string]any, h *models.AppHelmComponentConfig) {
	if h.ChartName != "" {
		doc["chart_name"] = h.ChartName
	}
	if h.Namespace != "" {
		doc["namespace"] = h.Namespace
	}
	if h.StorageDriver != "" {
		doc["storage_driver"] = h.StorageDriver
	}
	if h.TakeOwnership {
		doc["take_ownership"] = true
	}
	if len(h.Values) > 0 {
		values := make(map[string]any, len(h.Values))
		for k, v := range h.Values {
			values[k] = v
		}
		doc["values"] = values
	}
	if files := convertValuesFiles(h.ValuesFiles); files != nil {
		doc["values_file"] = files
	}
	convertVCS(doc, h.PublicGitVcsConfig, h.ConnectedGithubVcsConfig)

	if h.HelmConfigJSON != nil && h.HelmConfigJSON.HelmRepoConfig != nil {
		repo := make(map[string]any)
		rc := h.HelmConfigJSON.HelmRepoConfig
		if rc.RepoURL != "" {
			repo["repo_url"] = rc.RepoURL
		}
		if rc.Chart != "" {
			repo["chart"] = rc.Chart
		}
		if rc.Version != "" {
			repo["version"] = rc.Version
		}
		if len(repo) > 0 {
			doc["helm_repo"] = repo
		}
	}
}

func convertDockerBuild(doc map[string]any, d *models.AppDockerBuildComponentConfig) {
	if d.Dockerfile != "" {
		doc["dockerfile"] = d.Dockerfile
	}
	if d.Target != "" {
		doc["target"] = d.Target
	}
	if len(d.EnvVars) > 0 {
		envVars := make(map[string]any, len(d.EnvVars))
		for k, v := range d.EnvVars {
			envVars[k] = v
		}
		doc["env_vars"] = envVars
	}
	if len(d.BuildArgs) > 0 {
		args := make([]any, len(d.BuildArgs))
		for i, a := range d.BuildArgs {
			args[i] = a
		}
		doc["build_args"] = args
	}
	convertVCS(doc, d.PublicGitVcsConfig, d.ConnectedGithubVcsConfig)
}

func convertExternalImage(doc map[string]any, e *models.AppExternalImageComponentConfig) {
	if e.AwsEcrImageConfig != nil {
		ecr := make(map[string]any)
		if e.ImageURL != "" {
			ecr["image_url"] = e.ImageURL
		}
		if e.Tag != "" {
			ecr["tag"] = e.Tag
		}
		if e.AwsEcrImageConfig.AwsRegion != "" {
			ecr["region"] = e.AwsEcrImageConfig.AwsRegion
		}
		if e.AwsEcrImageConfig.IamRoleArn != "" {
			ecr["iam_role_arn"] = e.AwsEcrImageConfig.IamRoleArn
		}
		if len(ecr) > 0 {
			doc["aws_ecr"] = ecr
		}
	} else if e.ImageURL != "" || e.Tag != "" {
		pub := make(map[string]any)
		if e.ImageURL != "" {
			pub["image_url"] = e.ImageURL
		}
		if e.Tag != "" {
			pub["tag"] = e.Tag
		}
		doc["public"] = pub
	}
}

func convertJob(doc map[string]any, j *models.AppJobComponentConfig) {
	if j.ImageURL != "" {
		doc["image_url"] = j.ImageURL
	}
	if j.Tag != "" {
		doc["tag"] = j.Tag
	}
	if len(j.Cmd) > 0 {
		cmd := make([]any, len(j.Cmd))
		for i, c := range j.Cmd {
			cmd[i] = c
		}
		doc["cmd"] = cmd
	}
	if len(j.Args) > 0 {
		args := make([]any, len(j.Args))
		for i, a := range j.Args {
			args[i] = a
		}
		doc["args"] = args
	}
	if len(j.EnvVars) > 0 {
		envVars := make(map[string]any, len(j.EnvVars))
		for k, v := range j.EnvVars {
			envVars[k] = v
		}
		doc["env_vars"] = envVars
	}
}

func convertKubernetesManifest(doc map[string]any, k *models.AppKubernetesManifestComponentConfig) {
	if k.Namespace != "" {
		doc["namespace"] = k.Namespace
	}
	if k.Manifest != "" {
		doc["manifest"] = k.Manifest
	}
	// Kustomize is an embedded struct
	kustom := k.Kustomize
	kustomMap := toMap(&kustom)
	if len(kustomMap) > 0 {
		doc["kustomize"] = kustomMap
	}

	// PublicGitVcsConfig is an embedded struct too
	pub := k.PublicGitVcsConfig
	pubMap := toMap(&pub)
	if repo := pubMap["repo"]; repo != nil && repo != "" {
		vcsDoc := make(map[string]any)
		if r, ok := pubMap["repo"].(string); ok && r != "" {
			vcsDoc["repo"] = r
		}
		if d, ok := pubMap["directory"].(string); ok && d != "" {
			vcsDoc["directory"] = d
		}
		if b, ok := pubMap["branch"].(string); ok && b != "" {
			vcsDoc["branch"] = b
		}
		if len(vcsDoc) > 0 {
			doc["public_repo"] = vcsDoc
		}
	}

	if k.ConnectedGithubVcsConfig != nil {
		convertVCS(doc, nil, k.ConnectedGithubVcsConfig)
	}
}

func convertAction(a *models.AppActionWorkflowConfig) map[string]any {
	doc := make(map[string]any)

	if a.Timeout > 0 {
		doc["timeout"] = formatDuration(a.Timeout)
	}
	if a.BreakGlassRoleArn != "" {
		doc["break_glass_role_arn"] = a.BreakGlassRoleArn
	}

	if len(a.Triggers) > 0 {
		var triggers []any
		for _, t := range a.Triggers {
			m := make(map[string]any)
			if t.Type != "" {
				m["type"] = t.Type
			}
			if t.CronSchedule != "" {
				m["cron_schedule"] = t.CronSchedule
			}
			if len(m) > 0 {
				triggers = append(triggers, m)
			}
		}
		if len(triggers) > 0 {
			doc["triggers"] = triggers
		}
	}

	if len(a.Steps) > 0 {
		var steps []any
		for _, s := range a.Steps {
			m := make(map[string]any)
			if s.Name != "" {
				m["name"] = s.Name
			}
			if s.InlineContents != "" {
				m["inline_contents"] = s.InlineContents
			}
			if s.Command != "" {
				m["command"] = s.Command
			}
			if len(s.EnvVars) > 0 {
				envVars := make(map[string]any, len(s.EnvVars))
				for k, v := range s.EnvVars {
					envVars[k] = v
				}
				m["env_vars"] = envVars
			}
			if len(m) > 0 {
				steps = append(steps, m)
			}
		}
		if len(steps) > 0 {
			doc["steps"] = steps
		}
	}

	if len(a.ComponentDependencyIds) > 0 {
		deps := make([]any, len(a.ComponentDependencyIds))
		for i, d := range a.ComponentDependencyIds {
			deps[i] = d
		}
		doc["dependencies"] = deps
	}

	return doc
}

// prettyJSON re-formats JSON with 4-space indentation to match local file conventions.
// Returns the input unchanged if it's not valid JSON.
func prettyJSON(data []byte) []byte {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return data
	}
	pretty, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return data
	}
	pretty = append(pretty, '\n')
	return pretty
}

// formatDuration converts nanoseconds to a human-readable duration string.
func formatDuration(ns int64) string {
	d := time.Duration(ns)
	if h := d.Hours(); h >= 1 && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(h))
	}
	if m := d.Minutes(); m >= 1 && d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(m))
	}
	if s := d.Seconds(); s >= 1 && d%time.Second == 0 {
		return fmt.Sprintf("%ds", int(s))
	}
	return d.String()
}
