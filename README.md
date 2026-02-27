# nuon-ext-overlays

Kustomize-style config overlays for Nuon app configurations.

## Overview

Config overlays let you declaratively toggle platform features on/off without editing individual component or install TOML files. Define a single `overlay.toml` with patches that target config sections using selectors.

For multi-cloud apps, use **bases** to share common config (components, actions, policies) across cloud-specific app directories — no symlinks required.

## Install

```bash
nuon ext install nuonco/nuon-ext-overlays
```

## Usage

```bash
# Generate a starter overlay.toml from your app config
nuon overlays init --dir ./my-app

# Preview what changes the overlay would make
nuon overlays preview --dir ./my-app -f overlay.toml

# Apply overlay and write patched config
nuon overlays apply --dir ./my-app -f overlay.toml

# Validate overlay syntax and selectors
nuon overlays validate --dir ./my-app -f overlay.toml

# Compare local config against live API config
nuon overlays compare -d ./my-app
```

## Overlay Format

```toml
version = "1"

# Optional: inherit config from shared base directories
bases = ["../shared"]

# Disable drift on all components
[[patches]]
target = "components[*]"
[patches.set]
drift_schedule = ""

# Auto-approve the dev install
[[patches]]
target = 'installs[name="dev"]'
[patches.set]
approval_option = "approve-all"

# Remove all policies
[[patches]]
target = "policies"
strategy = "delete"
```

## Bases (Config Composition)

Use `bases` to compose an app config from a shared directory plus cloud-specific overrides. This replaces symlinks for multi-cloud setups.

```
my-app/
├── overlay-aws.toml         # bases = ["./shared"]
├── overlay-gcp.toml         # bases = ["./shared"]
├── shared/                  # common config
│   ├── components/
│   │   ├── monitoring.toml
│   │   └── operator.toml
│   ├── actions/
│   │   └── admin_token.toml
│   └── policies/
│       ├── rbac-view.yml
│       └── rbac-manage.yml
├── app-aws/                 # only AWS-specific config
│   ├── components/
│   │   ├── load_balancer.aws.toml
│   │   └── storage.aws.toml
│   ├── sandbox.toml
│   └── policies.toml
└── app-gcp/                 # only GCP-specific config
    ├── components/
    │   ├── load_balancer.gcp.toml
    │   └── storage.gcp.toml
    ├── sandbox.toml
    └── policies.toml
```

Overlay files live at the project root, keeping app directories clean. Running `nuon overlays apply` composes the full config:

```bash
nuon overlays apply -d ./app-aws -f ./overlay-aws.toml -o /tmp/app-aws-full
nuon overlays apply -d ./app-gcp -f ./overlay-gcp.toml -o /tmp/app-gcp-full
```

The output at `/tmp/app-aws-full` contains both the shared components (monitoring, operator) and the AWS-specific components (load_balancer, storage), plus shared policies and actions — all without symlinks.

### Precedence

When the same relative file path exists in both a base and the main directory, the main directory wins (file-level override). Bases are applied in order, with later bases taking precedence over earlier ones.

Duplicate `name` fields within components, actions, or installs across sources will produce an error.

## Selectors

| Selector | Matches |
|----------|---------|
| `components[*]` | All components |
| `components[name="X"]` | Component named X |
| `components[type="helm_chart"]` | All helm chart components |
| `installs[*]` | All installs |
| `installs[name="dev"]` | Install named "dev" |
| `sandbox` | Sandbox config |
| `runner` | Runner config |
| `policies` | Policies config |
| `inputs` | Inputs config |

## Patch Strategies

- **`merge`** (default): Deep-merge `set` fields into the target
- **`replace`**: Replace the entire section with `value`
- **`delete`**: Remove the section entirely

## Commands

### `preview`

Show what the overlay would change (colored diff). Optionally write the result:

```bash
nuon overlays preview -d ./my-app -f overlay.toml -o /tmp/patched
```

### `apply`

Apply overlay and write patched config to an output directory:

```bash
nuon overlays apply -d ./my-app -f overlay.toml -o /tmp/patched
```

### `validate`

Check overlay syntax and verify selectors match targets in the config:

```bash
nuon overlays validate -d ./my-app -f overlay.toml
```

### `init`

Generate a starter overlay.toml from an existing config directory:

```bash
nuon overlays init -d ./my-app
```

### `compare`

Compare local app config against the live config from the Nuon API. The local directory is parsed and normalized (file references are inlined, dependencies inferred, cloud-specific formats converted), then compared section-by-section against what the API returns.


```bash
nuon overlays compare -d ./my-app
```

Local-only files like `metadata.toml` and `installer.toml` are automatically excluded since they have no API equivalent.

TOML files are compared by parsed data (ignoring key order, comments, whitespace). Non-TOML files (policies, assets) are compared byte-for-byte (JSON assets are compared semantically).

Output:

```
Comparing local (./my-app) vs live API config

✓ components/monitoring.toml (toml)
✓ components/load_balancer.aws.toml (toml)
✓ policies/rbac-view.yml (asset)
✗ sandbox.toml (toml)
    type: local(aws-eks) vs live(gcp-gke)
⚠ installer.toml — only in local (toml)

5/6 files match, 1 mismatch
```

Exit code is `1` if any mismatches exist, `0` if everything matches — useful for CI.

## Examples

### Basic — Patching a single app

The [`examples/basic/`](examples/basic/) directory shows patching a single app config to disable drift, auto-approve dev installs, and remove policies:

```bash
cd examples/basic
nuon overlays preview -f overlay-dev.toml
```

### Multi-cloud — Shared config with bases

The [`examples/multi-cloud/`](examples/multi-cloud/) directory demonstrates sharing config across AWS and GCP apps without symlinks:

```
examples/multi-cloud/
├── overlay-aws.toml        # bases = ["./shared"]
├── overlay-gcp.toml        # bases = ["./shared"]
├── shared/                 # common components, actions, policies
│   ├── components/
│   ├── actions/
│   └── policies/
├── app-aws/                # only AWS-specific config
│   ├── components/
│   └── ...
└── app-gcp/                # only GCP-specific config
    ├── components/
    └── ...
```

Compose the full AWS config:

```bash
nuon overlays apply -d examples/multi-cloud/app-aws -f examples/multi-cloud/overlay-aws.toml -o /tmp/aws
```

Compose the full GCP config:

```bash
nuon overlays apply -d examples/multi-cloud/app-gcp -f examples/multi-cloud/overlay-gcp.toml -o /tmp/gcp
```

Both outputs will contain the shared monitoring and operator components alongside their cloud-specific load balancer and storage components.
