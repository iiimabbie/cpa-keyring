<div align="center">
  <img src="logo.png" width="120" alt="keyring logo">

  # keyring

  *替 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 的每把 key 取名字、綁帳號、限模型*

  [![Release](https://img.shields.io/github/v/release/iiimabbie/cpa-keyring?style=flat-square)](https://github.com/iiimabbie/cpa-keyring/releases)
  [![CLIProxyAPI](https://img.shields.io/badge/CLIProxyAPI-v8.0.60-3c873a?style=flat-square)](https://github.com/router-for-me/CLIProxyAPI)
  [![License](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](LICENSE)

  [功能](#功能) • [安裝](#安裝) • [使用](#使用) • [限制](#限制) • [English](README.md)

</div>

CLIProxyAPI 對每把客戶端 key 一視同仁，任何 key 都能用任何上游帳號和任何模型。keyring 讓你替每把 key 取名字、寫備註，指定它只能走哪些帳號、只能叫哪些模型，全部在 CPA 管理中心裡的頁面完成。

## 功能

- **名稱與備註**：替每把 key 標上名字。名字存在伺服器上，重啟、換瀏覽器或清掉網站資料都不會不見。
- **綁定帳號**：讓 key 只走指定的上游帳號。指定的帳號都不能用時直接報錯，不會偷偷改走別的帳號。
- **限制模型**：用 `*`、`?` 萬用字元設定允許或禁止的模型。禁止優先；允許清單留空代表全部可用。
- **模型清單過濾**：`/v1/models` 只列出這把 key 能用的模型。
- **管理頁**：從管理中心的插件選單打開，沿用原本的登入，頁面上的 key 會遮罩顯示。
- **檢查更新**：頁面右上角的按鈕會拿目前版本和 GitHub 上最新的 release 比對。
- **即裝即用**：規則存在 CPA 自己的 `config.yaml`，改了不用重啟，也不需要另外跑資料庫或其他服務。

沒有設定規則的 key，行為跟原本完全一樣。所有欄位都空白的 key 不會寫進設定檔。

## 安裝

1. 從 [Releases](https://github.com/iiimabbie/cpa-keyring/releases) 下載對應平台的 zip。提供 Linux amd64／arm64、macOS amd64／arm64 和 Windows amd64。
2. 解壓到 CPA 的插件目錄，`plugins/` 或 `plugins/<os>/<arch>/` 都可以（例如 `plugins/linux/amd64/`）。
3. 確認 `config.yaml` 有開啟插件，再重啟 CPA：

   ```yaml
   plugins:
     enabled: true
     dir: plugins
   ```

CPA 載入後，管理中心的插件選單就會出現 keyring。

## 使用

從插件選單打開 **keyring**，頁面會列出你的客戶端 key 和上游帳號。每把 key 可以填名字和備註、勾選能用的帳號、填入允許或禁止的模型。按下儲存就會寫進 `config.yaml`，CPA 馬上套用。

也可以直接改設定檔：

```yaml
plugins:
  configs:
    keyring:
      enabled: true
      keys:
        - scope: "<64 字元的 caller scope>"
          name: A 組
          note: 設計組共用
          auths: ["account-a.json", "team-*"]
          allow_models: ["gpt-*", "claude-*"]
          deny_models: ["*-preview"]
```

`scope` 用來識別 key，不必把 key 明文寫進設定。它是 `cli-proxy-api:caller-scope:v1\x00` 接上 key 之後算出的 SHA-256（小寫十六進位），用管理頁設定時會自動填好。`auths` 可以填 auth 檔名、auth ID 或萬用字元。

## 限制

以下都是 CLIProxyAPI v8.0.60 本身的限制。實際呼叫模型時一定會檢查，所以只影響 key 看得到什麼，或帳號綁定怎麼挑。

> [!NOTE]
> - **用網址參數驗證的模型清單**：如果客戶端呼叫 `/v1/models` 時只用 `?key=` 或 `?auth_token=` 驗證，CPA 不會告訴插件是誰在呼叫，清單就不會過濾。用 header 帶 key 才會過濾。
> - **查詢單一模型**：`GET /v1beta/models/{name}` 不經過插件過濾，被禁止的模型仍查得到，但呼叫一樣會被擋。
> - **同一個請求帶多把 key**：清單過濾會用第一個對得上 keyring 規則的 header。每個請求只帶一把 key，撤銷的 key 也記得刪掉規則。

> [!WARNING]
> - **一次只有一個排程插件**：CPA 只會執行優先度最高的排程插件。如果其他排程插件優先，keyring 的帳號綁定就不會生效。
> - **Home 模式**：Home 模式用另一條路挑帳號，管理頁也會停用，不要依賴這時的帳號綁定。
> - **寫在設定檔裡的 OpenAI 相容 key**：管理 API 不會列出這類帳號，頁面上無法勾選，請手動在 `auths` 填 `openai-compatibility:<provider 名稱>:*`，provider 名稱用小寫。
> - **不是絕對保證**：如果 CPA 呼叫排程時沒帶呼叫者身分，或插件連續失敗被 CPA 略過，請求會改由 CPA 原本的分流處理。

## 從原始碼建置

需要 Go 1.26 和 C 編譯器。

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -o keyring.so .
go test ./...
```
