<div align="center">
  <img src="logo.png" width="120" alt="keyring logo">

  # keyring

  *Per-key names, account bindings and model rules for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)*

  [![Release](https://img.shields.io/github/v/release/iiimabbie/cpa-keyring?style=flat-square)](https://github.com/iiimabbie/cpa-keyring/releases)
  [![CLIProxyAPI](https://img.shields.io/badge/CLIProxyAPI-v8.0.60-3c873a?style=flat-square)](https://github.com/router-for-me/CLIProxyAPI)
  [![License](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](LICENSE)

  [Features](#features) • [Installation](#installation) • [Usage](#usage) • [Limitations](#limitations) • [繁體中文](README.zh-TW.md)

</div>

CLIProxyAPI treats every client key the same: any key can use any upstream account and any model. keyring lets you give each key its own name and note, pin it to specific accounts, and limit which models it can call. Everything is managed from a page inside the CPA management center.

## Features

- **Names and notes**: label each client key. Names are saved on the server, so they survive restarts, other browsers and cleared site data.
- **Account binding**: pin a key to one or more upstream accounts. If none of them is available, the request fails instead of quietly falling back to another account.
- **Model rules**: allow or deny models per key with `*` and `?` wildcards. Deny wins, and an empty allow list means every model is allowed.
- **Filtered model list**: `/v1/models` only shows the models a key is allowed to use.
- **Management page**: opens from the plugin menu in the management center and reuses your existing login. Client keys are masked on the page.
- **Update check**: a button in the page header compares your version with the latest GitHub release.
- **No extra setup**: rules live in CPA's own `config.yaml` and reload without a restart. There is no database or other service to run.

Keys without a rule behave exactly as before. A key whose fields are all empty is not written to the configuration.

## Installation

1. Download the zip for your platform from [Releases](https://github.com/iiimabbie/cpa-keyring/releases). Builds are provided for Linux amd64/arm64, macOS amd64/arm64 and Windows amd64.
2. Extract it into your CPA plugin directory, either `plugins/` or `plugins/<os>/<arch>/` (for example `plugins/linux/amd64/`).
3. Make sure plugins are enabled in `config.yaml`, then restart CPA:

   ```yaml
   plugins:
     enabled: true
     dir: plugins
   ```

keyring appears in the plugin menu of the management center once CPA has loaded it.

## Usage

Open **keyring** from the plugin menu. The page lists your client keys and upstream accounts. For each key you can set a name and note, tick the accounts it may use, and fill in allowed or denied models. Saving writes the rules to `config.yaml` and CPA applies them right away.

You can also edit the configuration by hand:

```yaml
plugins:
  configs:
    keyring:
      enabled: true
      keys:
        - scope: "<64-character caller scope>"
          name: Team A
          note: Shared by the design team
          auths: ["account-a.json", "team-*"]
          allow_models: ["gpt-*", "claude-*"]
          deny_models: ["*-preview"]
```

`scope` identifies the key without storing it in plain text. It is the lowercase hex SHA-256 of `cli-proxy-api:caller-scope:v1\x00` followed by the key. The management page fills it in for you. `auths` accepts auth file names, auth IDs or wildcard patterns.

## Limitations

These come from CLIProxyAPI v8.0.60 itself. Actual model calls are always checked, so they only affect what a key can *see* or how bindings are picked.

> [!NOTE]
> - **Model list via query key**: if a client authenticates `/v1/models` only with `?key=` or `?auth_token=`, CPA does not tell the plugin who is calling, so the list is returned unfiltered. With a header key it is filtered.
> - **Single model lookup**: `GET /v1beta/models/{name}` skips plugin filtering, so a denied model can still be looked up by name. Calling it is still blocked.
> - **Multiple credentials in one request**: list filtering uses the first header that matches a keyring rule. Send one key per request and remove rules for revoked keys.

> [!WARNING]
> - **One scheduler at a time**: CPA runs only the highest-priority scheduler plugin. If another scheduler plugin wins, keyring's account bindings do not run.
> - **Home mode**: Home mode picks accounts through a different path and disables the management page. Do not rely on bindings there.
> - **Config-based OpenAI-compatible keys** are not listed by the management API, so they cannot be ticked on the page. Add them to `auths` by hand as `openai-compatibility:<provider name>:*`, with the provider name in lowercase.
> - **Not a hard guarantee**: if CPA calls the scheduler without the caller's identity, or bypasses the plugin after repeated failures, the request is handled by CPA's normal routing.

## Building from source

Requires Go 1.26 and a C compiler.

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -o keyring.so .
go test ./...
```
