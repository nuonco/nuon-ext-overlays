# nuon-ext-overlays

Kustomize-style config overlays for Nuon app configurations.

## Overview

Config overlays let you declaratively toggle platform features on/off without editing individual component or install TOML files. Define a single `overlay.toml` with patches that target config sections using selectors.

## Install

```bash
nuon ext install nuonco/nuon-ext-overlays
```

## Usage

```bash
# Generate a starter overlay.toml from your app config
nuon overlays init --dir ./my-app

# Preview what changes the overlay would make
nuon overlays preview --dir ./my-app -o overlay.toml

# Apply overlay and write patched config
nuon overlays apply --dir ./my-app -o overlay.toml

# Validate overlay syntax and selectors
nuon overlays validate --dir ./my-app -o overlay.toml
```

## Overlay Format

```toml
version = "1"

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

# Replace policies with a specific subset
[[patches]]
target = "policies"
strategy = "replace"
[patches.value]
[[patches.value.policies]]
name = "block-mutable-tags"
contents = "./block-mutable-tags.rego"
```

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

## Example

The [`example-app/`](example-app/) directory contains a full working example you can try locally:

```
example-app/
├── nuon.toml          # root config (sandbox, runner)
├── components/
│   ├── api.toml       # helm chart component
│   └── worker.toml    # helm chart component
├── installs/
│   ├── dev.toml       # dev install
│   └── prod.toml      # prod install
└── policies.toml      # OPA policies
└── overlay.toml       # disable drift, auto-approve dev, drop policies
```

Preview what the overlay would change:

```bash
cd example-app
nuon overlays preview -f overlay-dev.toml
```

Apply an overlay and write patched config to a directory:

```bash
nuon overlays preview -f overlay-dev.toml -o /tmp/patched
```

An example overlay might:

1. **Disable drift** on every component (`components[*]` → `drift_schedule = ""`)
2. **Auto-approve** the `dev` install (`installs[name="dev"]` → `approval_option = "approve-all"`)
3. **Remove all policies** (`policies` → `strategy = "delete"`)
