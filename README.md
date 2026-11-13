# keyring

A native CLIProxyAPI plugin for per-client names, notes, model policies, and upstream credential bindings. The implementation targets CLIProxyAPI v8.0.60. Policies use CPA's SHA-256 `caller_scope`, never a plaintext client key. Deny patterns take precedence; an empty allow list permits all models. `*` and `?` match arbitrary characters. A binding chooses the first available matching credential in the highest priority tier and rejects when no matching candidate remains. An unconfigured caller delegates scheduling to CPA.

## Configuration

Put `keyring.so` in the CPA plugin directory and configure CPA. Configure policies through the management resource page or compute a scope as the lowercase hexadecimal SHA-256 of the bytes `cli-proxy-api:caller-scope:v1\x00` followed by the client key.

```yaml
access:
  api-keys: [example-client-key]
plugins:
  enabled: true
  dir: /plugins
  configs:
    keyring:
      enabled: true
      priority: 0
      keys:
        - scope: "<64-character caller scope>"
          name: Example
          note: Team account
          auths: ["account-a.json", "team-*"]
          allow_models: ["model-*", "vendor/*"]
          deny_models: ["model-secret"]
```

The single policy source is `plugins.configs.keyring`; CPA persists it and reloads changes. The browser resource `/v0/resource/plugins/keyring/open` is available in the CPAMC plugin resources menu. It uses `/v8/management` to read client keys and credentials and to save the plugin configuration; it reuses a remembered same-origin CPAMC session when available, or accepts a management key held only in page memory. The page masks client keys. The `auths` field also accepts IDs and patterns manually.

## Known limitations

- CPA v8.0.60 does not give the model-list response interceptor the authenticated caller scope or query parameters. If a list request authenticates only through `?key=` or `?auth_token=` and has no valid client key header, the list is returned unfiltered. Actual model execution still applies the request interceptor. A future CPA that supplies `metadata.caller_scope` to this interceptor will be used automatically.
- When caller scope metadata is absent, the first header matching a configured keyring rule determines list filtering; CPA does not expose which header authenticated. With multiple credentials, especially a revoked key whose rule remains, the chosen rule can differ from the authenticated caller and expose a broader list. Remove stale rules and avoid sending multiple credentials. Model execution still uses the authenticated caller scope.
- `GET /v1beta/models/{name}` bypasses response interceptors in v8.0.60. A filtered model can still be viewed through this endpoint.
- Only one scheduler plugin is active: CPA chooses the highest priority scheduler. A different scheduler can prevent keyring bindings from running.
- Home mode disables local management and plugin resources; Home dispatch also chooses credentials through a separate path, not the local plugin scheduler. Do not rely on keyring account bindings in Home mode. Home mode has not been integration-tested.
- `/v8/management/credentials` lists file-backed credentials but does not list the config-backed OpenAI-compatible keys observed in testing. Those keys cannot currently be selected with checkboxes; their auth IDs must be supplied manually.
- If a scheduler call lacks CPA's caller scope, the plugin delegates to CPA because it cannot distinguish a bound caller from an unconfigured caller. CPA can also bypass/fuse a plugin after host-side failures. These cases do not provide an unconditional fail-closed guarantee.
- The release workflow builds Linux and macOS on amd64/arm64 and Windows on amd64. Windows arm64 is omitted because the workflow does not provision a Go-compatible Windows ARM64 C cross-compiler. The workflow and non-Linux binaries have not been run here.

## 繁體中文

keyring 以 CPA 的 caller scope 管理客戶端金鑰的名稱、備註、上游帳號綁定與模型白／黑名單。設定由 CPA 保存並熱更新，管理頁使用 v8 管理 API。僅透過 query key 驗證的模型清單無法取得呼叫者身分，因此不會篩選；實際模型呼叫仍會檢查。Home 模式的帳號綁定不可依賴，且管理頁不可用。設定型上游帳號目前不能在頁面勾選，請見上方限制。

## Build

With Go 1.26 and a C compiler: `CGO_ENABLED=1 go build -buildmode=c-shared -o keyring.so .`. Run `go test ./...` for unit tests.
