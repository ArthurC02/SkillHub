<h1 align="center">Skill Hub</h1>

<p align="center">
  尋找、建立、試跑與散布 Agent Skill 的開放平台。
</p>

<p align="center">
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <a href="https://github.com/ArthurC02/SkillHub/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/ArthurC02/SkillHub/actions/workflows/ci.yml/badge.svg"></a>
  <a href="README.md">English</a>
</p>

## 它做什麼

- **目錄與搜尋**——對已發布的 Skill 做關鍵字與語意搜尋，兩者合併成同一份排序。
- **隔離試跑**——每次執行都在隔離沙箱裡，並且留下它的軌跡與花費。
- **評估**——每個版本有測試案例與評審結果，讓「變好了」是量出來的而不是感覺。
- **版本不可變與可攜套件**——發布過的版本不再改動；Skill 可以匯出成套件，帶著授權、來源與（你要的話）測試案例一起走。
- **引導式創作**——用一句話描述需求就能起草一個 Skill。**預設關閉。**
- **淨測試模式**——同一套系統，跑在不能安裝軟體、也上不了網的機器上。見[下方](#淨測試模式)。

## 快速開始

最快看到產品跑起來的方式是**淨測試模式**：一個指令，不需要 Docker、不需要金鑰、不花錢。

```bash
task bootstrap                    # Go、npm 與 uv 的相依
npm ci --prefix tools/pglite      # 內嵌的 PostgreSQL 承載
npm --prefix apps/web run build   # 這個模式自己供應這份建置結果
task clean-mode                   # 全部啟動，然後印出網址
```

`task clean-mode` 會起三個行程，並印出 `open http://127.0.0.1:8080/`。缺東西時它會拒絕啟動，直接指名缺什麼、該下哪一條指令，所以你也可以先跑它、照它說的做。

畫面本身馬上就能用；附帶的示範 Skill 要等模型服務也起著才會進到目錄——套件在被增強索引之前不會出現在任何查詢裡。

沒有 [Task](https://taskfile.dev/) 也可以：每個 task 都有等價的原生指令，例如 `go -C tools/devctl run . bootstrap` 與 `node tools/cleanmode/start.mjs --seed`。

## 環境需求

| 工具 | 用在哪 |
| --- | --- |
| Go | 控制平面與沙箱提供者 |
| Node.js | 前端、淨模式啟動器與資料庫承載 |
| [uv](https://docs.astral.sh/uv/) | Python 服務與它的直譯器 |
| Docker | 開發用的資料庫、物件儲存與模型閘道 |
| [Task](https://taskfile.dev/) | 可選；每個 task 都有等價的原生指令 |

版本的權威來源是工具實際會讀的那幾個檔——`go.mod`、`.node-version`、`apps/llm/.python-version` 與 [`tools/toolchain.yaml`](tools/toolchain.yaml)——不寫在說明文字裡。`task doctor` 會拿你的機器跟它們逐項比對並指出哪裡不合。

也可以用[開發容器](.devcontainer/README.md)，裡面所有工具都已釘版本並裝好；淨測試模式則只需要 Node 與 Go。

## 跑起整套系統

淨測試模式換掉了三個實作。要跑真的那一套，先起基礎設施，再起四個服務：

```bash
task doctor      # 版本與前置需求
task env:init    # 從 .env.example 建立 .env，已存在就不覆寫
task bootstrap   # 相依；同時把 git 的 hooksPath 指到 .githooks
task dev         # Postgres 與 SeaweedFS 容器；不需要 secret、不花錢
```

| 服務 | 指令 | 埠 |
| --- | --- | --- |
| 控制平面 API（Go） | `go -C apps/platform run ./cmd/api` | 8080 |
| 佇列 Worker（Go） | `go -C apps/platform run ./cmd/worker` | — |
| 模型服務（Python） | 在 `apps/llm` 下 `uv run uvicorn skillhub_llm.app:app` | 8000 |
| 沙箱提供者（Go） | `go -C apps/sandbox run ./cmd/sandboxd` | 9000 |
| 前端（React） | `npm --prefix apps/web run dev` | 5173 |

API 需要 `DATABASE_URL`；沙箱提供者缺 `SKILLHUB_SANDBOX_TOKEN` 會直接拒絕啟動。[`.env.example`](.env.example) 列出所有變數——真正的值填進被 ignore 的 `.env`，永遠不要填進範例檔。

模型呼叫走一個**預設關閉**的閘道。`task dev:model` 會啟動它並檢查必要金鑰是否齊全；在那之後任何打到供應商的呼叫都會產生費用。本節其餘一切都免費。

前端要打本機 API 時，API 行程還需要 `DEV_CORS_ORIGIN=http://localhost:5173`：開發時兩者是不同來源、正式環境是同源，所以這個放行是逐行程選擇性開啟的。

## 淨測試模式

Skill Hub 平常需要真的資料庫、真的物件儲存與真的隔離沙箱。淨測試模式是**同一支程式**，把這三樣換成跑在行程內的替身：編譯成 WebAssembly 的 PostgreSQL、記憶體內的物件儲存，以及本機行程的執行 Driver。它存在的理由是：有些展示發生在不能安裝軟體、也沒有一般對外網路的機器上。

```bash
task clean-mode
```

> [!WARNING]
> 這個模式刻意比正式環境弱，而且會在畫面上說出來：**沙箱沒有任何隔離**、物件儲存**不驗證** presigned URL、資料庫**只有一條連線**，因此併發行為與正式環境不同。不要拿它跑不受信任的 Skill 或真實資料。

不設旗標就什麼都不會變：沒有 `SKILLHUB_CLEAN_MODE` 時，程式組出來的就是正式環境那一套接線。

## 專案結構

| 路徑 | 內容 |
| --- | --- |
| `apps/` | 四個可部署的程式：`web`、`platform`、`llm`、`sandbox` |
| `packages/` | 供其他程式 import 的 library，含生成的 API client |
| `contracts/` | 所有跨程序介面的唯一來源（OpenAPI、事件、封裝格式） |
| `db/` | Migration、查詢與資料庫測試 |
| `infra/` | Compose、runtime 映像、網路與可觀測性 |
| `tools/` | 開發、CI 與維運指令 |
| `docs/` | 架構決策、計畫與 runbook |

## 文件

- [架構決策](docs/adr/README.md)——系統為什麼長這樣。
- [開發手冊](docs/development/automation.md)——環境設定、程式碼生成、CI 與疑難排解。
- [Runbook](docs/runbooks/)——出事的時候看這裡。
- [`AGENTS.md`](AGENTS.md)——本專案的慣例與鐵律，寫給 coding agent，對人一樣有效。

## 參與貢獻

歡迎開 issue 與 pull request。動手前請讀 [`AGENTS.md`](AGENTS.md) 了解本專案強制的慣例，並在本機跑一次 `task ci`——那就是 CI 會跑的同一段確定性、不需要 secret 的流程。

## 授權

[MIT](LICENSE)
