package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nuonco/nuon/sdks/nuon-go/models"

	"github.com/nuonco/nuon-ext-overlays/internal/config"
	"github.com/nuonco/nuon-ext-overlays/internal/patcher"
)

// Client wraps the Nuon API SDK for fetching live app config.
type Client struct {
	apiURL   string
	apiToken string
	orgID    string
	appID    string
	http     *http.Client
}

// NewClient creates an API client from extension config.
func NewClient(cfg *config.Config) (*Client, error) {
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("NUON_API_TOKEN is required")
	}
	if cfg.OrgID == "" {
		return nil, fmt.Errorf("NUON_ORG_ID is required")
	}
	if cfg.AppID == "" {
		return nil, fmt.Errorf("NUON_APP_ID is required")
	}

	return &Client{
		apiURL:   cfg.APIURL,
		apiToken: cfg.APIToken,
		orgID:    cfg.OrgID,
		appID:    cfg.AppID,
		http:     &http.Client{},
	}, nil
}

// FetchLiveConfig fetches the latest app config from the API using the V2
// configs endpoint with recurse=true, and converts it to a ConfigBundle.
// Sub-configs (policies, permissions, secrets) that aren't hydrated by the
// recurse endpoint are fetched via their dedicated latest-* endpoints.
func (c *Client) FetchLiveConfig(ctx context.Context) (*patcher.ConfigBundle, error) {
	// Step 1: get latest config to obtain the config ID.
	latestCfg, err := c.getLatestConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Step 2: fetch full hydrated config via V2 endpoint.
	appConfig, rawJSON, err := c.getConfigV2(ctx, latestCfg.ID)
	if err != nil {
		return nil, err
	}

	// Step 3: hydrate action workflow configs with steps/triggers.
	// The recurse endpoint doesn't preload these nested associations.
	for i, awc := range appConfig.ActionWorkflowConfigs {
		if awc.ID == "" {
			continue
		}
		hydrated, err := c.getActionWorkflowConfig(ctx, awc.ID)
		if err != nil {
			continue // best-effort; fall back to un-hydrated config
		}
		appConfig.ActionWorkflowConfigs[i] = hydrated
	}

	// Step 4: backfill sub-configs not hydrated by recurse=true.
	if appConfig.Permissions == nil {
		if p, err := c.getLatestSubConfig(ctx, "permissions"); err == nil {
			var cfg models.AppAppPermissionsConfig
			if json.Unmarshal(p, &cfg) == nil {
				appConfig.Permissions = &cfg
			}
		}
	}
	if appConfig.Secrets == nil {
		if p, err := c.getLatestSubConfig(ctx, "secrets"); err == nil {
			var cfg models.AppAppSecretsConfig
			if json.Unmarshal(p, &cfg) == nil {
				appConfig.Secrets = &cfg
			}
		}
	}

	return convertAppConfig(appConfig, rawJSON)
}

func (c *Client) doGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("X-Nuon-Org-ID", c.orgID)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API %s: %s", path, string(body))
	}
	return body, nil
}

func (c *Client) getLatestConfig(ctx context.Context) (*models.AppAppConfig, error) {
	body, err := c.doGet(ctx, fmt.Sprintf("/v1/apps/%s/latest-config", c.appID))
	if err != nil {
		return nil, fmt.Errorf("fetching latest config: %w", err)
	}
	var cfg models.AppAppConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("decoding latest config: %w", err)
	}
	return &cfg, nil
}

// getLatestSubConfig fetches a sub-config (policies, permissions, secrets) via
// the dedicated latest-app-{name}-config endpoint. Returns raw JSON bytes.
// A 404 is treated as "not configured" and returns nil, nil.
func (c *Client) getLatestSubConfig(ctx context.Context, name string) ([]byte, error) {
	return c.doGet(ctx, fmt.Sprintf("/v1/apps/%s/latest-app-%s-config", c.appID, name))
}

func (c *Client) getActionWorkflowConfig(ctx context.Context, configID string) (*models.AppActionWorkflowConfig, error) {
	body, err := c.doGet(ctx, fmt.Sprintf("/v1/action-workflows/configs/%s", configID))
	if err != nil {
		return nil, fmt.Errorf("fetching action workflow config: %w", err)
	}
	var cfg models.AppActionWorkflowConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("decoding action workflow config: %w", err)
	}
	return &cfg, nil
}

func (c *Client) getConfigV2(ctx context.Context, configID string) (*models.AppAppConfig, []byte, error) {
	body, err := c.doGet(ctx, fmt.Sprintf("/v1/apps/%s/configs/%s?recurse=true", c.appID, configID))
	if err != nil {
		return nil, nil, fmt.Errorf("fetching config v2: %w", err)
	}
	var cfg models.AppAppConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, nil, fmt.Errorf("decoding config v2: %w", err)
	}
	return &cfg, body, nil
}

// actionState is used to parse action names from the app config state JSON.
type actionState struct {
	Actions []struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"actions"`
}

func parseActionNames(state string) map[string]string {
	if state == "" {
		return nil
	}
	var s actionState
	if err := json.Unmarshal([]byte(state), &s); err != nil {
		return nil
	}
	m := make(map[string]string, len(s.Actions))
	for _, a := range s.Actions {
		m[a.ID] = a.Name
	}
	return m
}

// rawPoliciesEnvelope extracts the nested policies array from the raw API
// response, which the SDK model (AppAppPoliciesConfig) doesn't capture.
type rawPoliciesEnvelope struct {
	Policies *struct {
		Policies []rawPolicy `json:"policies"`
	} `json:"policies"`
}

type rawPolicy struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Engine   string `json:"engine"`
	Contents string `json:"contents"`
}

// convertAppConfig transforms an API AppAppConfig into a ConfigBundle
// with TOML-style keys matching local config file structure.
// rawJSON is the raw V2 response used to extract data the SDK model doesn't capture.
func convertAppConfig(cfg *models.AppAppConfig, rawJSON []byte) (*patcher.ConfigBundle, error) {
	bundle := &patcher.ConfigBundle{
		Toml:   make(patcher.ConfigDir),
		Assets: make(map[string]patcher.Asset),
	}

	if cfg.Stack != nil {
		if doc := convertStack(cfg.Stack); len(doc) > 0 {
			bundle.Toml["stack.toml"] = doc
		}
	}

	if cfg.Sandbox != nil {
		bundle.Toml["sandbox.toml"] = convertSandbox(cfg.Sandbox)
	}

	if cfg.Runner != nil {
		bundle.Toml["runner.toml"] = convertRunner(cfg.Runner)
	}

	if cfg.Input != nil {
		if doc := convertInput(cfg.Input); len(doc) > 0 {
			bundle.Toml["inputs.toml"] = doc
		}
	}

	// Policies: the SDK model is incomplete (missing nested policies array),
	// so we parse from the raw JSON response instead.
	if doc := convertPoliciesFromJSON(rawJSON); len(doc) > 0 {
		bundle.Toml["policies.toml"] = doc
	}

	if cfg.Permissions != nil {
		convertPermissions(cfg.Permissions, bundle)
	}

	if cfg.Secrets != nil {
		if doc := convertSecrets(cfg.Secrets); len(doc) > 0 {
			bundle.Toml["secrets.toml"] = doc
		}
	}

	if cfg.BreakGlass != nil {
		if doc := convertBreakGlass(cfg.BreakGlass); len(doc) > 0 {
			bundle.Toml["break_glass.toml"] = doc
		}
	}

	// Build component ID → name mapping for dependency resolution.
	compNames := make(map[string]string, len(cfg.ComponentConfigConnections))
	for _, comp := range cfg.ComponentConfigConnections {
		if comp.ComponentID != "" && comp.ComponentName != "" {
			compNames[comp.ComponentID] = comp.ComponentName
		}
	}

	for _, comp := range cfg.ComponentConfigConnections {
		name := comp.ComponentName
		if name == "" {
			name = comp.ComponentID
		}
		key := "components/" + name + ".toml"
		bundle.Toml[key] = convertComponent(comp, compNames)
	}

	actionNames := parseActionNames(cfg.State)

	for _, action := range cfg.ActionWorkflowConfigs {
		name := action.ActionWorkflowID
		if resolved, ok := actionNames[name]; ok {
			name = resolved
		}
		key := "actions/" + name + ".toml"
		doc := convertAction(action)
		doc["name"] = name
		bundle.Toml[key] = doc
	}

	if cfg.Readme != "" {
		bundle.Assets["README.md"] = patcher.Asset{Bytes: []byte(cfg.Readme)}
	}

	return bundle, nil
}
