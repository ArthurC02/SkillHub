# ADR-022：Sandbox 部署拓撲與安全驗收定值

- 狀態：**Accepted**（2026-08-16 定案）
- 日期：2026-08-16
- 決策者：產品負責人、架構規劃
- 相關：[ADR-005](./ADR-005-self-hosted-sandbox-baseline.md)（最低安全基線）、[ADR-015](./ADR-015-sandbox-isolation-technology.md)（gVisor 基線、獨立 VM 池）、[ADR-017](./ADR-017-model-gateway-and-llm-observability.md)（模型閘道）、[ADR-018](./ADR-018-containerized-core-infrastructure.md)（E1 容器化、Hetzner）、[ADR-019](./ADR-019-monorepo-structure-and-cicd.md)（CI/CD 與 runner）、`02:SEC-002`／`SEC-009`、[m0/threat-model-and-sandbox-baseline.md](../plans/mvp/m0/threat-model-and-sandbox-baseline.md)（45 項基線、Q1～Q4／Q18）

## 背景

ADR-015 定了隔離技術（gVisor）與平面邊界（獨立 VM 池、不進 Kubernetes），但把三件事留在待決策，而這三件事各自阻擋一組安全驗收：

1. **節點編排的具體形式**（威脅模型 Q1）——決定 45 項基線多數檢查的**驗證形式**。
2. **節點是否單租戶**（Q2）——直接決定 TM-EXE-01（逃逸）與 TM-EXE-05（Run 間殘留）的殘餘風險大小。
3. **Egress Proxy 實作與允許清單管理流程**（Q3，ADR-005 與 ADR-015 皆列為待決策）——N-01～N-07 全部依賴它；允許清單是威脅模型 Top 2 風險的核心控制。

同時 `02:SEC-002` 有**六項無值語句**（威脅模型 Q18）：節點重建週期、gVisor 安全基準版本與更新 SLA、映像掃描結果有效期、漏洞等級門檻、Reconciler 掃描頻率、遺留資源告警與暫停門檻。無值＝檢查存在但不可自動化判定，依 `02:SEC-002`「未定值前不得記為通過」，六項連同 Q1～Q3 一起把 SEC-002、SBX-002、SBX-007 全部卡在未勾。

M2 已完結，`services/sandbox` 的 dev 形態（DockerProvider ＋ `internal: true` 網路上只有 LiteLLM 閘道）在真實端到端 Run 上驗證過。本 ADR 要做的是把 dev 形態的**語意**寫成生產形態，並把六項門檻變成值。

**本 ADR 不重開 ADR-015 的隔離技術選擇，也不重開 ADR-018 的平台選擇。** gVisor 基線、獨立 VM 池、不進 Kubernetes、Hetzner Cloud 都是既有決策；本文件只回答「那具體長什麼樣、什麼數字算通過」。

定值的共同約束：**封閉測試規模**（20 使用者／50 Run 日／尖峰併發 2 slot／2 台 Sandbox 節點，[cost-estimation.md](../plans/mvp/m0/cost-estimation.md) §2.2），維運人力為每月 8–13 h 的單人團隊（同文件 §5.4）。**一個做不到的值等於沒有值**——本文件每一項門檻都附「違反時的動作」，而每個動作都必須是這個規模的團隊真的做得到的動作。做不到的地方一律走 fail-closed（停用比假裝守得住好），不寫企業級 SLA。

---

# 第一部分：部署拓撲選型（Q1～Q3）

## Q1：節點編排

### 評估選項

#### 選項 A：compose-per-VM（採用）

每台 Sandbox 節點跑 Docker Engine（`containerd` ＋ `runsc` runtime）＋ **一份 compose 檔、一個服務**（`sandboxd`）。節點以 IaC（cloud-init ＋ 該 compose 檔）建置，無叢集、無 control plane、無服務發現。

- 優點：
  - **不引入第二個排程器**。派工決策已經在控制平面：RUN-005 依 `GET /capability` 的空 slot 與 `egress_modes` 選節點。叢集排程器會與它重疊，而重疊的兩個排程器必然有一個是錯的（誰算 slot？誰決定 drain？）。
  - 與 ADR-018 的 E1 同一套工具（compose），節點維運不新增技術面。
  - 節點無狀態、無成員關係，**「定期重建」就只是「砍掉重開一台」**——這正是 P-03 要的性質，任何有叢集成員狀態的方案都會讓重建變成「先退出叢集」。
  - 閘門 A（節點准入）可以是一支在開機時跑完就上報的探針，不必表達成某個編排器的 admission 概念。
- 缺點：節點清單是靜態設定（`SKILLHUB_SANDBOX_PROVIDERS`），加減節點要改控制平面設定並重啟；節點數多時人工成本線性上升。

#### 選項 B：k3s（單一 Sandbox 叢集，RuntimeClass=runsc）

- 優點：宣告式、滾動更新、節點健康與 drain 現成；與 ADR-018 若日後走 E2 的路徑一致。
- 缺點：**ADR-015 與 ADR-018 例外 3 已明文排除**——把執行平面放進編排層會讓 ADR-001 的平面隔離退化為命名空間隔離。即使另開一個獨立叢集（不與應用共用），也是為了 2 台節點養一套 API Server、etcd 與版本升級稅（ADR-018 已算過：每季 1.3–5.3 h），而它提供的排程能力與 RUN-005 重複。**否決。**

#### 選項 C：Nomad

- 優點：單一 binary，比 k8s 輕；docker driver 直接支援指定 runtime；不強制 CNI 那一整套。
- 缺點：仍是一個**叢集**（server raft ＋ client agent）與一個**排程器**，選項 B 的兩個核心缺點原封不動——只是稅比較便宜。而且它是團隊完全沒有的第三套技術，違反 ADR-014 選型原則 2（產品數量最少，該原則在 ADR-018 延續有效）。**否決。**

### 決策

**採選項 A：compose-per-VM。**

| 項目 | 決定 |
| --- | --- |
| 節點軟體 | Ubuntu LTS ＋ Docker Engine ＋ containerd ＋ `runsc`（gVisor），`daemon.json` 註冊 `runsc` runtime、`--icc=false`、`--iptables=true` |
| 節點上的服務 | **只有 `sandboxd` 一個**（`services/sandbox`），compose `restart: unless-stopped`，掛 `docker.sock`（它是 Provider，不是工作負載） |
| 建置方式 | IaC：cloud-init 腳本 ＋ `infra/compose/sandbox-node.yml` ＋ `infra/egress/allowlist.yaml`（見 Q3）。**節點不接受手動修改**——要改就重建 |
| 排程 | 控制平面 RUN-005，唯一。節點只回報 `GET /capability` |
| 節點註冊 | 靜態設定（`SKILLHUB_SANDBOX_PROVIDERS` ＋ 每節點 token） |
| 入站 | 只有 `:9000`（sandboxd），來源限控制平面節點 IP；其餘全部 DROP。SSH 走供應商 console 或限來源 IP，且**不得**是部署流程的一部分 |

**有效範圍**：本決策以**封閉測試的 2 台節點**為前提。ADR-015 的早期成長情境（500 Run/日 ＝ 13 slot ＝ **5 台節點**）**已經越線**——靜態節點清單、手動加減節點與逐台跑 Suite 2 在 5 台上仍勉強可行但已無餘裕。**到達早期成長規模時必須重開本節評估，不得默認沿用。**

**重評條件（觸發即重開編排評估）**：Sandbox 節點數 > 4（與 ADR-018 的 E1→E2 觸發條件 2 同一門檻，理由相同：手動編排成本超過學習成本），或需要按 Workspace／地區分池而靜態設定表達不了。屆時的候選順序是 Nomad 優先於 k3s——因為 ADR-015 的平面隔離理由排除的是「與應用共用編排層」，獨立叢集只是成本問題，而 Nomad 比較便宜。

## Q2：節點租戶模型

### 評估選項

#### 選項 A：每個 Run 一台專用 VM（per-run 單租戶）

- 優點：逃逸的爆炸半徑就是那一次 Run 自己，TM-EXE-01／TM-EXE-05 的殘餘風險趨近於零。
- 缺點：**與已定案的成本模型不相容**。ADR-015 定案紀錄的容量模型是「每節點 slot 數 ＝ `min(vCPU÷2, GiB÷4)`」，早期成長情境 13 slot ＝ 5 台 8 vCPU 節點；改成 per-run 就是 13 台以上，Sandbox 池本來就佔平台成本 83%。更致命的是**時間**：成本模型只給 provisioning ＋ gVisor 冷啟 1 分鐘，開一台 VM 做不到。**否決**（保留為 ADR-015 對沖路徑的一部分：若威脅模型升級，正確動作是換 MicroVM Provider，不是把 VM 當容器開）。

#### 選項 B：節點單租戶於「執行平面」，同節點多 Run 併存（採用）

- 優點：符合既有容量與成本模型；符合 ADR-015「獨立 VM 池、不與一般應用工作負載混排」的原文（該句約束的是**工作負載類別**，不是 Run 數）。
- 缺點：同節點的 Run 之間存在橫向風險——逃逸後可觸及同節點其他 Run 的記憶體與 tmpfs；且**東西向網路連通是預設行為**，不是逃逸才有的能力（見下方強制條件）。

#### 選項 C：節點按 Workspace 分池（per-workspace 單租戶）

- 優點：跨 Workspace 的橫向風險消失。
- 缺點：MVP 只有個人工作區、封測 20 人，等於回到選項 A 的成本結構卻只買到一半的隔離；且需要一套池管理邏輯（哪個 Workspace 對應哪個池、池空了怎麼辦），是純新增的複雜度。**否決。**

### 決策

**採選項 B。「單租戶」的單位明確定義為：節點只承載不受信任執行平面的工作負載，不與控制平面、資料平面或任何應用工作負載混排。單位是「平面」，不是 Run，也不是 Workspace。**

同節點多 Run 併存被接受，代價以下列**強制條件**壓住，每一條都對應 45 項基線的既有項或本 ADR 新增的驗收測項：

| # | 強制條件 | 依據／落點 |
| --- | --- | --- |
| 1 | 每個 Run 一個獨立 gVisor sandbox，`/work`／`/out` 為該容器私有 **tmpfs**（行程結束即由核心回收，不落盤）；不共用任何可寫層、Volume 或快取 | C-01、C-06（M2 已落地並驗證） |
| 2 | **同節點的 sandbox 之間必須網路不可達**。出口網路不得讓兩個 sandbox 互通——Docker 的預設 bridge 行為是同網段互通，`--icc=false` 加**每 Run 一個網路命名空間**是本 ADR 指定的作法 | 新增，補 N-02／N-05 的同節點面；SEC-009 測項 T5-4 |
| 3 | sandbox 不得連到 `sandboxd` 自己的 `:9000`、不得連到節點的 bridge gateway 位址、不得連到節點 loopback | 新增，補 N-03 的「控制平面位址」在同節點的具體形式；SEC-009 測項 T5-5 |
| 4 | 節點以 IaC 建置、無狀態、**7 天滾動重建**（見第二部分 P-03） | ADR-015、P-03 |
| 5 | 逃逸疑慮成立時的動作不是「隔離那台節點」，而是 **SEC-010 全池緊急停用** | ADR-015 定案紀錄、威脅模型 §5.5 |
| 6 | **沙箱層允許清單的目的地不得是控制平面節點的位址。** LiteLLM 閘道必須有**沙箱面專屬位址**（獨立小 VM 或獨立 IP），不得沿用它在控制平面節點上的位址——否則「唯一到得了的位址」就是控制平面本身，Q2 的節點單租戶與 ADR-001 的平面隔離同時被繞過 | 新增（Q3 硬條件）；SEC-009 測項 T5-7 |

**強制條件 6 的成本**：一台 Hetzner **CX33（4 vCPU/8 GiB）＝ $9.80 ＋ IPv4 $0.58 ≈ $10.38/月**。[cost-estimation.md](../plans/mvp/m0/cost-estimation.md) §4.1／§4.2 已經為「Egress Proxy 1 台小型（$10.38）」編列這筆錢，而 Q3 決定不部署 Proxy——**這條預算行改承載沙箱面的 LiteLLM 節點，總成本不變**。早期成長情境的 HA 兩台（$21）同理。

> ⚠️ **E1 若把 LiteLLM 與控制平面合併在同一節點（現行 compose 即是：`127.0.0.1:4000`），強制條件 6 不成立。** 這在 dev 可接受，**生產不可**。若基於成本仍決定合併，該決定必須在此明示為殘餘風險並回頭更新威脅模型 TM-CTL-01——本 ADR 的立場是不合併，$10.38 買的是 ADR-001 平面隔離不被一條防火牆規則抵銷。

**明示的殘餘風險**：一次成功的 gVisor 逃逸可觸及同節點其他 Run 的執行中資料。本 MVP 接受此風險，理由是 gVisor 逃逸的機率被 ADR-015 評為足夠低、封測資料敏感度有限、且緩解手段（7 天重建 ＋ 一鍵停用）存在。**這是一個被接受的風險，不是一個被解決的問題**；威脅模型 TM-EXE-01 的殘餘風險欄應據此更新（維護觸發條件：新增 Provider）。

## Q3：Egress Proxy 選型與允許清單管理

### 先確認事實：MVP 的沙箱允許清單只有一項，且是平台自有服務

M2 的實作把三個原本預期的目的地收斂成一個：

| 原預期目的地（ADR-015 定案紀錄） | M2 實際 | 為什麼 |
| --- | --- | --- |
| LiteLLM 閘道 | **沙箱直連**（唯一） | 鐵律 8：模型呼叫必須由工作負載自己發出 |
| 物件儲存短效授權端點 | **沙箱到不了** | SBX-008：sandboxd 代搬位元組，沙箱不持有任何預簽 URL |
| Trace ingestion 端點 | **沙箱到不了** | TRACE-002：工作負載寫 JSONL 到 `/out`，sandboxd 讀出後代推 |

因此**允許清單分兩層，信任等級不同，規則也不同**：

| 層 | 主體 | 信任等級 | 允許目的地 |
| --- | --- | --- | --- |
| **沙箱層** | Run 的 gVisor 容器 | T0 完全不受信任 | **LiteLLM 閘道，一項**。N-01～N-07 全部適用於這一層 |
| **節點層** | `sandboxd` 程序、`dockerd` 拉映像 | T2 平台服務 | 物件儲存端點、Trace ingestion 端點、容器映像 registry。同樣 default-deny，但不受 N-01～N-07 約束（ADR-005 的網路隔離章節，其適用對象是 Sandbox 出站） |

這個事實直接決定選型：**要用一個 L7 Proxy 去表達「你只能到一個我們自己跑的內部服務」，買到的政策能力沒有對應的需求**。

### 評估選項（沙箱層）

#### 選項 A：Squid

- 優點：`dstdomain` 允許清單就是政策本身，設定即可讀；`CONNECT` 白名單成熟；不解密內容（符合 N-06「不記錄內容」）；資源足跡小；access log 天然含目的地、方法與位元組數。
- 缺點：**它只看得到走它的流量**。default-deny 的強度取決於「沒有旁路」，而旁路是路由層的問題，Squid 管不到——N-02「無旁路」仍得靠 L3 規則保證，於是 L3 規則反正要寫。它**看不見被拒絕的非 HTTP 嘗試**（原始 socket、UDP、ICMP、對 RFC1918 的 TCP 掃描），而那些正是 TM-EXE-03 的主要偵測面。多一個要設定、要升級、要監控的元件。

#### 選項 B：Envoy

- 優點：L7 政策表達力最強、可觀測性最好、xDS 可動態下發允許清單。
- 缺點：它的價值在**動態設定與大量路由**；MVP 的允許清單是一項靜態內部位址，xDS 沒有東西可餵，等於用最重的工具做 Squid 都嫌大的事。**否決。**

#### 選項 C：nftables default-deny ＋ 節點固定 DNS 解析器（採用）

每個 Run 一個網路命名空間；**強制點在主機側**——nftables 的 `forward` 鏈（Docker 環境下即 `DOCKER-USER` 的等價位置）或該 Run 的 netns 內，預設 `drop` 並 `log`，只 `accept` 到「允許清單渲染出來的釘選 IP:port」。**不是容器內的 `output` 鏈**：容器內的規則由不受信任的工作負載所在的命名空間承載，能被逃逸後改掉；強制點必須在沙箱管不到的一側。DNS 走節點上的 dnsmasq，對允許清單內的名字**只有靜態記錄，其餘不遞迴、不轉發**。

- 優點：
  - **N-02「無旁路」由構造保證**——不是「流量被引導到 Proxy」，而是**除了那個位址以外沒有路由**。這與 dev 形態（`internal: true` 網路上只有閘道）是同一個語意，只是換成生產可用的強制手段。
  - **看得見被拒絕的嘗試**，含非 HTTP：`log` 的每一筆 drop 都帶目的地 IP:port、協定與方向。DNS tunneling、內網掃描、Metadata Service 存取全部在這一層變成可計數的事件（威脅模型 §5.3 的終止門檻因此有東西可數）。
  - **DNS Rebinding 結構性不可能**：回答是靜態的，而且就算回答被竄改，L3 規則只允許釘選 IP。DNS tunneling 同時被關掉（解析器不遞迴，沒有出口）。
  - 東西向阻擋（Q2 強制條件 2、3）與允許清單是**同一組規則**，不是兩套機制。
  - 零新元件：nftables 在 OS 裡，dnsmasq 是一個 30 KB 的靜態設定。
- 缺點：
  - 允許清單以 **IP:port** 表達，不是域名。目的地換 IP 就要重建節點——對「平台自有、IP 由自己控制」的目的地這是正確的取捨，對第三方 SaaS 就不是（見重評條件）。
  - 記錄粒度是 L3/L4，沒有 URL 或 SNI。**對一個只有一個目的地的允許清單，IP 即身分**，這個損失是零；一旦清單長出第二個第三方目的地，它就不是零了。

### 決策

**採選項 C：沙箱層以 nftables default-deny ＋ 節點固定 DNS 解析器實作，不部署 Squid／Envoy。**

生產形態與 dev 形態的對照（同語意，不同強制手段）：

| | dev（本機 Docker） | prod（執行節點） |
| --- | --- | --- |
| 「只有閘道到得了」 | `internal: true` 網路上只有 LiteLLM | 每 Run netns ＋ nftables 只 accept 釘選的閘道 IP:port |
| 沙箱之間 | **未阻擋（已知缺口）** | `--icc=false` ＋ 每 Run 獨立 netns，東西向 DROP |
| DNS | Docker 內嵌解析器 | 節點 dnsmasq，靜態記錄、不遞迴、不轉發 |
| 被拒絕的嘗試 | 看不見（沒有路由就沒有記錄） | nftables `log`，逐筆計數 |

> ⚠️ 上表第二列是本次分析發現的**既有 dev 缺口**：同一張 `skillhub_egress` 網路上的 sandbox 目前可以互相連通，這是**不需要逃逸**的跨 Run 橫向路徑。dev 環境一次只跑少量 Run 且無外部使用者，風險可接受；**生產不得複製這個形態**，故列為 Q2 強制條件 2 與 SEC-009 測項 T5-4。

### 節點層的強制形式與沙箱層不同

沙箱層的目的地是平台自有服務，IP 由我們控制，所以「釘選 IP」成立。**節點層（`sandboxd`、`dockerd`）的目的地全是第三方**——物件儲存、Trace ingestion（若平台入口在另一台）、容器映像 registry——它們的 IP 由供應商輪替，**釘選會在對方下一次部署時靜默斷線**。

| | 沙箱層（`tier: sandbox`） | 節點層（`tier: node`） |
| --- | --- | --- |
| 主體信任等級 | T0 完全不受信任 | T2 平台服務 |
| 強制形式 | default-deny ＋ **釘選 IP:port** | default-deny ＋ **FQDN 比對**（解析器對這些名字正常遞迴） |
| `pinned_ip` | **必填** | **不適用，不得填**（CI 以 warning 標記） |
| 適用 N-01～N-07 | 是 | 否（ADR-005 的網路隔離章節，其適用對象是 Sandbox 出站） |
| Q3 的重評條件 | **適用**——第二個目的地即觸發 | 不適用（節點層本來就是多個第三方目的地） |

**重評條件只約束 `tier: sandbox`。** 節點層加一個第三方目的地不代表沙箱層的「一項且平台自有」失效，也就不觸發 L7 Proxy 的重評；把兩者混在一起會讓重評條件永遠處於誤觸發狀態，於是沒有人再看它。

### 兩個把重評條件與允許清單變成閘門的 CI 斷言（A1-c）

紀律會安靜地停止發生，所以 Q3 的兩項保證各配一個斷言（[`.github/workflows/egress-allowlist.yml`](../../.github/workflows/egress-allowlist.yml) ＋ [`check_egress_allowlist.py`](../../tools/ci/check_egress_allowlist.py)）：

| 斷言 | 行為 |
| --- | --- |
| **重評條件機械化** | `tier: sandbox` 的條目數必須 **== 1** 且 `name == model_gateway`，否則 **fail** 並在訊息中明寫「**ADR-022 Q3 的重評條件已觸發**——允許清單不再是『一項且平台自有』，L7 Proxy 決策必須在這個 PR 落地前重開」。這讓「第二個目的地」不可能靜默加入 |
| **N-07 deny-list** | 任一 tier 的 `fqdn` 命中模型供應商網域（子字串比對，涵蓋區域與版本化主機）即 **fail**，訊息指向「在 LiteLLM 閘道內加供應商，不在這裡加」（鐵律 8）。**無例外路徑** |
| `tier: sandbox` 缺 `pinned_ip` | fail |
| `tier: node` 帶 `pinned_ip`，或 fqdn 是平台自有網域 | **warning**（不擋——節點層的判斷需要人看，但要看得到） |
| **威脅模型連動** | PR 動到 `infra/egress/**` 而未動 `plans/mvp/m0/threat-model-and-sandbox-baseline.md` → **fail**。把該文件 §3 的維護規則從紀律變成紅燈 |

### sandboxd 側的二次比對（A1-e）

節點的 nftables 是**強制**，但平台送來的 `RunRequest.egress.allow` 與節點實際渲染的清單可能不一致（例如節點是舊版設定、或平台指向了另一個閘道位址）。此時沙箱會被接上一張到不了目的地的網路，症狀是模型呼叫空等到逾時——M2 已實測過這個症狀（188 秒 `Request timed out`），而它看起來像 Skill 的問題。

因此：**`sandboxd` 在 `accept()` 時逐項比對 `RunRequest.egress.allow[].url` 與本節點已渲染的 `tier: sandbox` 清單，不相符即以 `capability_mismatch` 拒絕**（既有錯誤類別，RUN-001／契約 422）。快速失敗並歸因正確，好過一個註定逾時的 Run。SEC-009 測項 T9 加入此斷言。

**重評條件（觸發即重開 L7 Proxy 評估，首選 Squid）——只約束 `tier: sandbox`**：沙箱層允許清單新增**任何第二個目的地**（不論是否平台自有），或出現需要以域名而非 IP:port 表達的沙箱層規則，或需要 per-Run／per-Workspace 差異化的沙箱層清單。任一成立時 L3 規則會開始表達不了政策，而那是換工具的正確時機——不是現在。**此條件由 CI 斷言強制**（見下方 A1-c），不靠人記得。

### 允許清單的存放處與變更流程

| 項目 | 決定 |
| --- | --- |
| **存放處** | repo 內單一檔案 **[`infra/egress/allowlist.yaml`](../../infra/egress/allowlist.yaml)**（本 commit 已建立骨架），是 nftables 規則與 dnsmasq 記錄的**唯一來源**，由 IaC 在節點建置時渲染。節點上的規則檔是產物，人工修改節點即為設定漂移，下次重建（≤ 7 天）自動抹除 |
| **每筆欄位** | `tier`（`sandbox`／`node`，強制形式見上表）、`name`、`fqdn`、`pinned_ip`（**`tier: sandbox` 必填；`tier: node` 不適用不得填**）、`port`、`protocol`、`justification`（為什麼這個目的地必須存在）、`owner`、`review_by` |
| **變更流程** | ①**獨立 PR**，只改這個檔（不與功能變更混在一起，否則審查會滑過去）→ ②**產品負責人核准**；MVP 單人團隊下，核准的形式即 PR 合併紀錄，commit message 必須載明目的地、理由與威脅模型影響 → ③**同一個 PR 必須一併更新** [threat-model-and-sandbox-baseline.md](../plans/mvp/m0/threat-model-and-sandbox-baseline.md)（其 §3 維護規則明文：「開放新的 egress 目的地時必須更新」）→ ④合併後**由節點重建套用**，不熱改執行中的節點 |
| **CI 強制** | 見下方 A1-c 的五項斷言（重評條件、N-07 deny-list、`pinned_ip` tier 規則、威脅模型連動） |
| **複審** | 每筆 `review_by` ≤ **90 天**（與映像豁免清單同節奏）。逾期 → 告警，且該檔進入**凍結**（不得再新增任何目的地）直到複審完成；**逾期不自動移除既有項目**——自動移除模型閘道會讓整個平台停擺，那是把運維疏失升級成事故 |
| **N-07 的硬規則** | 允許清單**不得**出現任何模型供應商網域（`api.openai.com` 等），任一 tier 皆然。這一條沒有例外流程：需要新供應商就在 LiteLLM 閘道裡加，不在這個檔裡加。CI 以 deny-list 斷言（鐵律 8） |
| **未部署前的狀態** | `pinned_ip: unset`。IaC 以 `unset` 渲染**產不出 accept 規則**——沙箱因此什麼都到不了，這是 fail-closed 的方向，不是待補的 TODO |

### 目的地記錄（N-06）

| 項目 | 決定 |
| --- | --- |
| 記錄什麼 | 目的地 IP:port、協定、方向、決策（accept／drop）、位元組數、時間、關聯鍵 |
| 關聯鍵 | 每 Run 的網路命名空間（或容器 ID），由 sandboxd 在回報時對應成 `run_id`。**不在封包層帶 workspace**（鐵律 3：workspace 一律由平台自 `run_id` 查） |
| 不記錄什麼 | 內容、Header、URL、任何 payload（N-06 明文；也讓這份記錄不需要遮罩） |
| 保存期 | **90 天**，與 PDM-006 的 Trace 保存期同級。純 L3/L4 記錄，量極小 |
| 用途 | ①威脅模型 §5.3 的「執行中 egress 阻擋事件達門檻即終止 Run」有東西可數（門檻本身是 Q4，仍未定值，見待決策）；②事故調查；③O11Y-003 的「Egress 阻擋」指標來源 |

---

## 補充決策：Container registry 採 GHCR（回答 [ADR-019](./ADR-019-monorepo-structure-and-cicd.md) 待決策 1）

本決策原不在本 ADR 範圍，但它是 I-03／I-04 的結構性阻擋：SBOM 與掃描時戳沒有地方隨 image 保存，閘門 A 就查不到，SEC-009 的「0 unknown」因此**結構性不可達**。不定 registry 就等於把一個上線關卡永久掛住。

### 評估選項

| 選項 | 優點 | 缺點 |
| --- | --- | --- |
| **GHCR（採用）** | repo 已在 GitHub、CI 已在 Actions，推送用 `GITHUB_TOKEN` 即可，不需管理額外憑證；**OCI referrer／`cosign attach` 支援成熟**，SBOM 與掃描 attestation 可作為 referrer 隨 image digest 保存——這正是 I-03／I-04 缺的落點；digest pin 天然（ADR-019 job 5 已用 commit SHA 當 tag）；私有 repo 的儲存與頻寬在 GitHub 方案內 | 與 GitHub 綁定加深（但 repo 與 CI 本來就在那裡，這不是新增的依賴） |
| Docker Hub | 生態最廣 | 匿名拉取有速率限制，節點拉映像會踩到；還要另管一組憑證；attestation 支援落後 |
| 部署平台自帶／自架 registry | 與節點同內網、拉取最快 | Hetzner 無受管 registry；自架 ＝ 又一個要備份、要升級、要監控的資料產品，違反 ADR-014 選型原則 2（該原則在 ADR-018 延續有效） |

### 決策

**採 GHCR。** 連帶成立：

- **I-03**：SBOM 以 OCI referrer 隨 image digest 保存（`cosign attach sbom` 或 `oras attach`），閘門 A 探針可用 digest 直接查詢，不再依賴 90 天的 CI artifact。
- **I-04**：`scanned_at` 成為 attestation 上的機器可讀欄位，30 天有效期首次可被自動判定；並可接 `schedule:` 觸發**重掃 registry 裡的 digest**（而非重掃當場建出來的 image），這是 `infra/images/README.md` 記錄為「目前做不到」的那一項。
- **SEC-009 的前置條件**因此可以寫死：**「受測的 Runtime Image 已發佈至 GHCR 且附 SBOM 與掃描 attestation」**——沒有這個前提，T8 的 I-03／I-04 只能是 `unknown`，而 `unknown` 視同 fail。

**未做**：發佈流水線接 GHCR push ＋ attestation 是實作工作，歸部署批，已在 `03-work-items.md` 第 11 節補承接工作項（SBX-011）。本 ADR 只定選型與它解開的驗收路徑。

---

# 第二部分：SEC-002 六項門檻定值

適用範圍：`02:SEC-002`「目前為無值語句的六項門檻…驗收前必須有值」。**定案日 2026-08-16，來源 ADR-022。**

定值的共同原則：值必須配得上封閉測試規模（2 台節點、4 slot、單人維運）；每一項都要有**誰在哪裡量**與**違反時做什麼**；做不到的一律 fail-closed。

## 1. P-03 節點重建週期

| | |
| --- | --- |
| **值** | **7 天**（節點自建立起算的最大壽命），滾動重建，一次一台。另有**事件觸發的立即重建**：gVisor 安全基準版本變更（P-04）、逃逸疑慮、該節點清理連續失敗（X-04） |
| **依據** | 威脅：TM-EXE-01 逃逸後在節點上的持久化——重建週期就是潛伏攻擊窗口的上界。7 天讓「一次成功逃逸」最多存活一週，而不是一季。<br>運維成本：節點無狀態、以 IaC 建置，重建＝銷毀＋重建＋跑閘門 A 探針。封測 2 台節點 ＝ 每月 8 次重建，落在 cost-estimation §5.4 的「節點 OS 修補與重建 2–3 h/月」內。N+1 冗餘讓一次一台的滾動重建不掉容量（1 台服務即滿足尖峰 2 slot）。<br>為什麼不是更短：每日重建的邊際安全收益遠小於「每天有一個變更窗口」帶來的操作風險，且封測規模下沒有自動化滾動部署可依靠。<br>為什麼不是更長：30 天會讓潛伏窗口與 gVisor 例行更新節奏（每月）脫鉤，等於安全基準版本平均落後半個月 |
| **量測** | 閘門 A 節點准入探針上報 `node_created_at`（來自 cloud-init 寫入的建置時戳，非節點自報的當下時間）；控制平面每次呼叫 `GET /capability` 時比對年齡 |
| **違反時** | 年齡 > 7 天 → **告警**（P-03 為告警級，不改）並自動排入重建佇列；> 14 天仍未重建 → 值班依 SEC-010 runbook **手動 drain** 該節點（不再接受新 Run，等現有 Run 結束後銷毀）。drain 是值班動作，不是自動閘門——自動 drain 在只有 2 台節點時可能一次拿掉半個池 |

## 2. P-04 gVisor 安全基準版本與更新 SLA

| | |
| --- | --- |
| **值** | **「安全基準版本」＝上游最新 release 或其前一版（N 或 N−1），且發佈日不早於 90 天前**。維護者：**平台維運負責人**（MVP 階段由產品負責人兼任）。記錄位置：`infra/egress/` 同層的 **`infra/nodes/gvisor-baseline.txt`**（單行版本字串，repo 內唯一來源，與允許清單同樣由 IaC 渲染進節點）。<br>**例行更新：每月一次**，併入 P-03 的重建週期（重建就是換版的手段，不另做熱升級）。<br>**CVE 應變 SLA**（自上游 advisory 公開起算）：<ul><li>**沙箱逃逸類**（上游標記為 sandbox escape／VMM escape，或 CVSS ≥ 9.0）：**24 小時**內完成全池換版；**做不到即依 SEC-010 停用 `SelfHostedProvider`**（fail-closed）</li><li>**High（CVSS 7.0–8.9）**：**7 天**</li><li>**Medium 以下**：隨下一次例行更新（≤ 30 天）</li></ul>訂閱來源：gVisor GitHub Security Advisories ＋ release notes，由維護者訂閱並每月確認訂閱仍有效 |
| **依據** | 威脅：TM-EXE-01 明列「gVisor 自身漏洞需持續跟隨上游修補」為持續責任；逃逸類 CVE 是唯一會讓 ADR-015 的整個論證失效的事件類別。<br>運維成本：N／N−1 而非「必須最新」，讓單人團隊不必在上游發版當天動作；90 天下界防止 N−1 在上游長期不發版時退化成無限舊。<br>**24 小時為什麼開得出來**：它不是「24 小時內一定修好」的承諾，而是「24 小時內要嘛換版、要嘛停機」——停用是一個一鍵動作（SEC-010），單人團隊做得到。開一個沒有 fail-closed 退路的 SLA 才是空頭支票 |
| **量測（兩個點，缺一不可）** | ①**節點面**：閘門 A 准入探針執行 `runsc --version`，與 `gvisor-baseline.txt` 比對；低於基準即不通過。<br>②**基準面**：[`.github/workflows/gvisor-baseline.yml`](../../.github/workflows/gvisor-baseline.yml) 每日 cron，以 `gh api` 讀 gVisor 的 releases 與 security advisories——基準檔不在 `{N, N−1}` 或最新 release 發佈日 > 90 天 → fail 並開 issue；24 小時內出現逃逸類 advisory（severity `critical` 或摘要含 `escape`）→ 開最高等級 issue。<br>**只有 ① 是不夠的**：探針回答「這台節點合乎基準」，沒有任何東西回答「基準本身還是不是最新的」——那原本是「人記得去讀上游發版」，而那正是 CVE SLA 最容易安靜停止運作的一環 |
| **24 小時起算點** | **該 cron job 開出 issue 的時刻**，不是上游 advisory 的發佈時刻。這讓 SLA 的起點是一個可稽核的時戳，而不是「有人讀到通知的那一刻」（後者不受控，是本 ADR 原本最軟的一環）。代價明示：cron 每日一次，最壞情況比上游晚 24 小時起算 |
| **違反時** | P-04 為**阻擋**級：該節點**不得加入執行池**；已在池中者標記 drain。全池皆低於基準（例如剛提高基準）→ 依威脅模型 §5.1，新 Run 停留 `queued` 並顯示「執行環境暫時不可用」，**不得降級放行** |

## 3. I-04 映像掃描結果有效期

| | |
| --- | --- |
| **值** | **30 天**（**採納 SBX-002 的提案值**，見 [infra/images/README.md](../../infra/images/README.md) §「門檻提案值」）。到期前 **7 天**告警；到期即該 Image digest **不得被新 Run 引用**（閘門 D） |
| **依據** | 採納 SBX-002 的原始論證（grype 漏洞資料庫每日更新、Debian security 修復節奏以週計，30 天是「不會每天吵人、又不至於讓一個已知可修的 High 跑一整季」的折衷），並補上兩點本 ADR 才能給的依據：<ul><li>與 P-03 的 7 天節點重建**刻意不同步**。節點重建換的是 OS 與 gVisor，映像重掃換的是 Runtime 內容，兩者的變更節奏與變更成本都不同；把它們綁在一起會讓每週的節點重建被一次映像重建拖住。</li><li>到期即封鎖是 fail-closed，代價是「沒人重建映像就會停機」。7 天的提前告警是給單人團隊的實際反應窗口——一次重建＋重掃是 CI 上的一次 PR</li></ul> |
| **量測** | 發佈記錄的 `scanned_at` 對比當下時間。**現況限制（誠實記錄）**：container registry 未定案（ADR-019 待決策 1），`scanned_at` 目前只存在於 CI artifact 與 `infra/images/README.md` 的「當前掃描結果」段落，**是人工維護的日期**。registry 定案後改為隨 image 存放的機器可讀 attestation（OCI referrer／`cosign attach`），由閘門 A 探針查詢。在那之前，這一項**不可自動化判定**，是 SBX-002 維持不勾的唯一剩餘原因 |
| **違反時** | 閘門 D：該 Image 不得發佈、不得被新 Run 引用。已在跑的 Run 不中斷（映像過期不等於映像有漏洞，中斷已在跑的 Run 沒有安全收益） |

## 4. I-06 漏洞等級門檻與例外放行流程

| | |
| --- | --- |
| **值** | **採納 SBX-002 的提案並補上例外流程的批准與時效**：<br><table><tr><td>發現</td><td>處置</td></tr><tr><td>Critical／High **且上游有修復版本**（`grype --only-fixed --fail-on high`）</td><td>**阻擋發佈**（CI fail）。**無豁免路徑**</td></tr><tr><td>Critical／High **且上游無修復**</td><td>逐項具名列入 `infra/images/README.md` 的豁免清單：CVE 編號 ＋ 為何無法處理 ＋ 緩解層 ＋ 複審日</td></tr><tr><td>Medium 以下</td><td>只記錄在 artifact，不擋</td></tr></table>**豁免複審日 ＝ `first_exempted_at` ＋ 90 天**（該 CVE **首次**被豁免的日期，逐項記在豁免清單裡），逾期即該豁免失效 → 該次掃描結論不再成立 → 依 I-04 視為過期（＝阻擋）。**批准者**：產品負責人；批准的形式是豁免清單的變更 commit，須為獨立 commit 且訊息載明 CVE 與理由 |
| **依據** | 採納 SBX-002 的核心論證，它是對的且不需要更好的替代：Debian 對多數 base OS CVE 標 `won't fix`，純 `--fail-on high` 會讓這個 job **永遠紅、且沒有任何人做得出讓它變綠的動作**——那不是閘門，是雜訊。分成「可修的擋、不可修的具名記錄」才讓紅燈重新等於「有事情要做」。實證支持：現行映像可修的 Critical／High 已由「移除 npm／corepack」歸零，證明這條政策確實會推動修復而非累積豁免。<br>本 ADR 補上的兩點：<ul><li>**「可修的 Critical／High 無豁免路徑」是刻意的**。一個跑不受信任程式碼的沙箱，對「上游已經給了修復而我們沒裝」沒有正當理由。留下 override 就會有人用。</li><li>**豁免用 I-04 咬人，不改 I-06 的等級**。I-06 在基線裡是告警級（威脅模型 v2 §8 修正 4 的決定），本 ADR 不原地改寫它；讓逾期豁免透過「掃描結論失效 → I-04 過期」變成阻擋，效果相同且不動基線。<br>兩個時效的分工：**30 天的重掃是機器的**（每次發佈自動重跑 grype，豁免清單自動重生），**90 天的複審是人的**（是否仍無修復、緩解層是否仍成立、該不該換 base 發行版）。<br>**錨定在 `first_exempted_at` 而非「掃描日」**：以掃描日錨定的話，每 30 天的例行重掃都會把複審日往後推 90 天，**人工複審永遠不會到期**——那個時效等於不存在</li></ul> |
| **量測** | `runtime-image` CI job 的門檻步驟（`grype --only-fixed --fail-on high`）；豁免清單的 `複審日` 由同一 job 比對當下日期 |
| **違反時** | 可修的 Critical／High → CI fail，Image 不得發佈。豁免逾期 → 依 I-04 判為掃描過期，該 digest 不得被新 Run 引用 |

## 5. X-02 Reconciler 掃描頻率

| | |
| --- | --- |
| **值** | **每 5 分鐘**一輪（＝ 既有的 `run.OrphanScanInterval`，本 ADR 只是把實作值升格為定值）。掃描對象：Sandbox 容器、Network Rule／netns、LiteLLM Virtual Key、物件短效授權。<br>**年齡寬限只適用於「平台不認得的 sandbox」，維持既有的 5 分鐘**（`run.orphanGrace`）——那是派工在途的窗口：attempt 列已存在但這次掃描讀到的快照早於它，在窗口內殺掉會誤殺健康的新 Run。<br>**平台已辨識且已終態的 sandbox 立即清除，無寬限**——那不是「可能還在跑」，那是確定該走而沒走的 |
| **依據** | 威脅：TM-EXE-05（Run 間殘留資料）與 TM-SEC-02（憑證在 TTL 內被超用）——遺留資源的存活時間就是這兩個威脅的窗口。<br>容量：封測只有 **4 個 slot**（2 節點 × 2 slot）。一個洩漏的 sandbox 就吃掉 25% 容量，5 分鐘的偵測延遲上界是可接受的最大值；再長就會出現「使用者排隊而節點看起來很閒」。<br>成本：一輪＝對每個節點一次 `GET /runs?active=true` ＋ 一次資料庫查詢，2 節點下可忽略；與既有的 Outbox publisher（River periodic，5 秒）同一機制，不新增基礎設施。<br>**兩種寬限分開，是對齊既有實作而非新增規則**（`services/platform/internal/run/cleanup.go` 的 `isOrphan`／`orphanByAge` 已經是這個語意）——把它們合成一條「一律等硬牆鐘＋5 分」會讓確定該清的資源多活十幾分鐘 |
| **量測** | River periodic job；每輪輸出「掃描到 N 筆、清理成功 M 筆、失敗 K 筆」的指標（O11Y-003） |
| **違反時** | X-02 為**阻擋**級，阻擋的是「清理視為完成」：Reconciler 未執行（job 停擺 > 2 輪 ＝ 10 分鐘無心跳）→ 告警最高嚴重度，並依威脅模型 §5.4 暫停新 Run 排入受影響節點池。**理由：沒有 Reconciler 在跑，就沒有人知道洩漏了多少** |

## 6. X-03／X-04 遺留資源的告警與暫停門檻

遺留資源分兩類，傷害不同，門檻也不同：

### 6a. 佔用 slot 的遺留資源（Sandbox 容器）——容量問題

| | |
| --- | --- |
| **X-03 告警** | **同一筆資源連續 2 輪掃描（＝10 分鐘）仍存在** → 告警。用「連續兩輪」而非「出現即告警」是為了消除清理與掃描的競態 |
| **X-04 暫停** | ①**單一節點**上的遺留資源 ≥ 該節點宣告 slot 數的 **50%**（下限 1 筆）→ **該節點停止接受新 Run（drain）**，其他節點不受影響；②**全池**遺留資源 ≥ 總 slot 數的 **25%**（**下限 2 筆**）→ **暫停整個節點池派送新 Run**，Run 停留 `queued` 並顯示「執行環境暫時不可用」，同時依 SEC-010 判斷是否走全域緊急停用 |
| **依據** | 以**比例**而非絕對數表達，才能同時對封測（4 slot）與早期成長（20 slot）成立。<br>套進封測規模：單節點 2 slot × 50% ＝ **1 筆**——一個清不掉的沙箱就讓該節點退場，而 N+1 冗餘讓剩下那台撐住尖峰 2 slot，這是把冗餘用在它該用的地方。<br>全池門檻的**下限 2 筆是必要的**：純 25% 在 4 slot 下等於 1 筆，一次洩漏就停掉整個平台——那不是保守，是把一個容量事件升級成全平台事故。套進早期成長：20 slot × 25% ＝ 5 筆。<br>兩個門檻的分工對應 ADR-005 原文「遺留資源超過門檻時**告警**，必要時**暫停新 Run**」：告警是通知，暫停是阻斷（威脅模型 v2 §8 修正 3 已對齊等級，本 ADR 只填值） |
| **量測** | Reconciler 維護一張 **in-flight orphan 表**（`provider_run_id → first_seen_round`），每輪比對上一輪的集合：連續 2 輪命中同一 `provider_run_id` 即告警；每輪另計算 `遺留筆數 ÷ 宣告 slot 數`，逐節點與全池各一個指標（O11Y-003）。<br>**現有 metrics 表達不了「連續 2 輪」**——`skillhub_orphan_sandbox_total` 是累加計數器，`increase(...) > 0` 只說得出「這段時間內銷毀過東西」，說不出「同一個東西還在」。**需要新指標與這張表，屬實作工作**（`03` 第 11 節 SBX-012），本 ADR 只定語意；`infra/observability/alerts.yml` 的規則在那之前以註解標明是過渡形式 |
| **違反時** | 如上（drain／暫停）。恢復條件：遺留筆數降到門檻以下且連續 2 輪維持，才自動恢復派送——**不做單輪恢復**，避免在清理不穩定時來回抖動。<br>**任一節點處於 drain 期間，暫停 P-03 的例行滾動重建**（事件觸發的緊急重建除外）：一邊 drain 一邊照表重建會讓可用節點數同時被兩件事扣減，而 N+1 只準備了一份餘裕 |

### 6b. 不佔 slot 的遺留資源（Virtual Key、Network Rule）——憑證窗口問題

| | |
| --- | --- |
| **值** | **連續 3 輪（＝15 分鐘）撤銷失敗** → **最高嚴重度告警**，並依 SEC-010 判斷是否停用 Provider。**不自動暫停派送** |
| **依據** | 威脅：TM-SEC-02，一把撤不掉的 Virtual Key 是一個持續開著的成本與外洩窗口。<br>**為什麼不暫停派送**：暫停新 Run 不會讓已經外洩的憑證消失——它是一個看起來像在做事、實際上沒有降低任何風險的動作。正確動作是人介入（在閘道側強制刪除，或依 SEC-010 停用）。<br>3 輪而非 2 輪：撤銷失敗最常見的原因是閘道短暫不可用，多給一輪重試比多發一次假警報划算 |
| **量測** | **分項計數器**：`gateway_revoke_failed`（LiteLLM `/key/delete` 失敗）與 `sandbox_destroy_failed`（Provider `DELETE` 失敗）各自獨立。**不以 `cleanup_status` 為事實來源**——它是整個 attempt 的清理狀態，任一子步驟失敗都會把它設成 `failed`，因此區分不出「金鑰撤不掉」（憑證窗口，走 6b）與「沙箱殺不掉」（容量問題，走 6a），而兩者的正確動作相反 |

> **物件短效授權不在本項內。** 預簽 URL **沒有撤銷手段**——簽出去就有效到 TTL 為止，Reconciler 對它無事可做。它的窗口上界因此是**簽發時的 TTL**（SBX-008：該 Run 硬牆鐘 ＋ 5 分鐘），這是**明示的殘餘風險而不是可監控的門檻**。要縮短窗口只能縮 TTL，或改用可撤銷的授權形式（例如經 sandboxd 代理的一次性下載），後者不在 MVP 範圍。
| **違反時** | 最高嚴重度告警 → SEC-010 runbook。**嚴重度分級與值班歸屬仍是威脅模型 Q17，未定案**——本 ADR 不代為定值，但把「這一項屬最高嚴重度」寫死，讓 Q17 定案時有明確的輸入 |

## 六項定值一覽

| # | 檢查 | 值 | 量測點 | 違反時 |
| --- | --- | --- | --- | --- |
| P-03 | 節點重建週期 | **7 天**滾動，＋事件觸發 | 閘門 A 探針比對 `node_created_at` | 告警＋排入重建；>14 天值班手動 drain |
| P-04 | gVisor 安全基準版本／SLA | **N 或 N−1 且 ≤ 90 天**；例行月更；逃逸類 CVE **24 h**（做不到即停用）、High **7 天**。24 h 自 cron 開出 issue 起算 | ①閘門 A 探針比對 `runsc --version`；②**每日 cron 比對上游 releases／advisories** | **阻擋**節點加入池；已在池者 drain |
| I-04 | 掃描結果有效期 | **30 天**（到期前 7 天告警） | GHCR 上隨 digest 的掃描 attestation（`scanned_at`） | 閘門 D：Image 不得發佈、不得被新 Run 引用 |
| I-06 | 漏洞等級門檻 | **可修的 Critical／High 阻擋（無豁免路徑）**；不可修者具名豁免，複審日 ＝ **`first_exempted_at`**＋90 天 | `runtime-image` CI job | CI fail；豁免逾期 → 依 I-04 判過期 |
| X-02 | Reconciler 掃描頻率 | **每 5 分鐘**；寬限只給「平台不認得的 sandbox」的 5 分鐘派工窗口，已辨識終態者立即清 | River periodic job 指標 | 停擺 >10 分 → 最高嚴重度告警＋暫停派送 |
| X-03／X-04 | 遺留資源告警／暫停 | 告警：同一 `provider_run_id` 連續 2 輪仍在。暫停：單節點 ≥50% slot（下限 1）drain；全池 ≥25% slot（**下限 2**）暫停派送，且 drain 期間暫停例行滾動重建。非 slot 類：連續 3 輪撤銷失敗 → 最高嚴重度告警，不暫停 | Reconciler 的 in-flight orphan 表 ＋ 分項失敗計數器（**需新指標，SBX-012**） | drain／暫停派送／SEC-010 |

---

# 第三部分：SEC-009／SBX-010 驗收程序

`02:SEC-009` 與 ADR-015 定案紀錄都把逃逸測試列為**上線前硬性關卡**，但兩處都只有一句話。本節把它變成部署批拿到就能執行的清單。**現在不跑；這是交付給部署批的程序。**

## 0. 一個影響 ADR-019 待決策 3 的前提修正

M2 README 與 `03:SBX-010` 的註記都寫「逃逸測試需要 **Linux ＋ 巢狀虛擬化**」。**gVisor 不需要巢狀虛擬化**：`runsc` 的預設平台是 `systrap`（以 seccomp ＋ `SIGSYS` 攔截系統呼叫），只有 `--platform=kvm` 才需要 `/dev/kvm`。這把 ADR-019 待決策 3（CI runner）的範圍從「必須自架 runner」縮小為「需要 Linux runner ＋ Docker ＋ 可安裝 runsc」，**GitHub-hosted `ubuntu-latest` 即可**。

> 這一點必須在部署批的第一台節點上實測確認（`runsc --platform=systrap` 起一個容器並跑完一次 Run），確認後才依此規劃 CI。**在實測確認前，本節的 Suite 1／Suite 2 分割是提案而非事實。**

## 1. 測試環境

| 項目 | 需求 |
| --- | --- |
| **Suite 1（可在 CI 跑）** | Linux（`ubuntu-latest` 或等價）、Docker、`runsc` 依上游安裝、無需 KVM、無需生產網路。涵蓋計算隔離、資源耗盡、Runtime 相容性、映像供應鏈、契約測試 |
| **Suite 2（只能在真實節點跑）** | **一台與生產同規格、由生產同一份 IaC 建置的 Sandbox 節點**（Hetzner CCX23 級，4 vCPU/16 GiB），已套用 Q3 的 nftables 規則與 dnsmasq 設定，可達控制平面與 LiteLLM 閘道。涵蓋節點與拓撲、網路隔離、憑證範圍、清理與遺留資源 |
| **重要約束** | Suite 2 **不使用專用測試機**，就是即將加入池的那台節點，**在加入池之前跑**。理由：測試的對象是節點的實際設定，換一台機器就換了受測物 |
| **前置條件（缺一即 T8 判 `unknown` ＝ fail）** | ①受測的 Runtime Image **已發佈至 GHCR 且附 SBOM 與掃描 attestation**（I-03／I-04 的可查詢來源，見「補充決策：Container registry」）；②`infra/nodes/gvisor-baseline.txt` 已填實際版本（非 `unset`）；③`infra/egress/allowlist.yaml` 的 `tier: sandbox` 條目 `pinned_ip` 已填實際位址，且該位址**不是控制平面節點**（Q2 強制條件 6） |
| **模型費用** | Runtime 相容性測項（T4）會跑 45 個精選 Skill 的完整 Run，實測單次全量約 **$3.4**（M2 基準試跑數據）。預算內，但不得每次 PR 都跑——T4 只在 Runtime Image 或 gVisor 版本變更時跑 |

## 2. 測項清單（覆蓋 45 項基線全部）

| # | 測項 | Suite | 覆蓋的基線檢查 | 通過判準 |
| --- | --- | --- | --- | --- |
| **T1** | **公開容器逃逸 PoC 集**。起始集合：runc host binary overwrite（CVE-2019-5736）、cgroup v1 `release_agent`（CVE-2022-0492）、runc 洩漏 fd／Leaky Vessels（CVE-2024-21626）、Dirty Pipe（CVE-2022-0847）；另加通用嘗試：寫 `/proc/sys/kernel/core_pattern`、`mount` syscall、載入核心模組、`ptrace` 主機 PID、存取 `/dev/mem`、尋找任何 unix socket | 1 | C-02～C-09、C-16、P-01 | **全數失敗，且節點側無可觀察副作用**：節點檔案系統無新增檔案、`dmesg` 無 taint、無以 uid 0 新起的程序、其他 slot 的 Run 不受影響。PoC 集**逐年更新**，不宣稱完整 |
| **T2** | **syscall 煙霧級 fuzz**：容器內以隨機參數呼叫 syscall，4 並行 × 30 分鐘 | 1 | C-08、C-09 | Sentry 不 crash、主機核心無 oops、容器內程序未取得任何非預期權限。**誠實標示：這是煙霧級，不是研究級 fuzzing**——研究級屬 gVisor 上游的責任，不是本平台買得起的東西 |
| **T3** | **資源耗盡**：fork bomb、記憶體壓力、tmpfs 填滿、FD 耗盡、無限迴圈撞牆鐘 | 1 | C-10～C-15 | 各限制由 runtime 強制生效（PID 上限拒絕、OOM kill 該容器而非節點、tmpfs 寫入失敗、`nofile` 拒絕、逾時強停）；**且同節點其他 Run 的完成時間劣化 < 20%**（這一條才是「限制有效」的真正判準——限制存在但節點被拖垮等於沒有限制） |
| **T4** | **Runtime 相容性**：45 個精選 Skill 的 M2 基準試跑在 `runsc` 上重跑 | 1 | C-09 | **終態與 runc 基準逐筆一致**；任何不一致必須逐筆歸因，且**不得歸因於 gVisor syscall 不相容**。不設容忍百分比——不一致就是要查。每次擴充 Runtime 或升級 gVisor 重跑 |
| **T5** | **網路外洩**，八個子項：<br>T5-1 DNS tunneling：對允許清單外的域名做 TXT 查詢<br>T5-2 內網掃描：對 RFC1918 /24 掃 1000 port<br>T5-3 Metadata Service：`169.254.169.254`<br>T5-4 **東西向**：同節點另一個 Run 的位址<br>T5-5 **節點面**：`sandboxd:9000`、bridge gateway、節點 loopback<br>T5-6 **N-07**：`api.openai.com` 等供應商網域<br>T5-7 **閘道位址的非允許 port**：對允許清單的 `pinned_ip` 連 5432（Postgres）、平台 API port 與 9000<br>T5-8 **入站**：自外部對節點掃 port | 2 | N-01～N-07 | 八項全部 **DROP**；T5-1 另須 dnsmasq 回 NXDOMAIN；每一次嘗試都**在 nftables 記錄中留下一筆 drop**（沒有記錄 ＝ N-06 未成立，即使流量確實被擋）。<br>**T5-7 是 Q2 強制條件 6 的驗證**：允許的是 `IP:port` 不是 `IP`，若該位址剛好是控制平面節點，這一項就會亮——它是「閘道不得與控制平面同址」被違反時唯一會叫的東西。<br>**T5-8 驗 N-05 與節點入站規則**：只有控制平面來源 IP 對 `:9000` 可通，其餘全 DROP。<br>反向驗證：到 LiteLLM 閘道釘選 `IP:port` 的連線**必須成功**（否則測的是「什麼都連不上」） |
| **T6** | **憑證範圍**：跨 Run 使用 Virtual Key、TTL 過期後使用、在容器內搜尋預簽 URL 特徵 | 2 | D-01～D-06 | 前兩者皆 **401**；容器的環境變數與整個可寫檔案系統 **grep 不到 `X-Amz-Signature`**（驗證 SBX-008 的「沙箱不持有任何預簽 URL」在生產形態仍成立） |
| **T7** | **清理失敗與遺留門檻**：`kill -9` sandboxd 後重啟、節點強制重啟、Run 執行中強制銷毀容器、重複 `destroy`；**另加人工注入假遺留資源**（在節點上直接建帶 `skillhub.sandbox` label 但平台不認得的容器） | 1（部分）／2 | D-07、X-01～X-04、C-16 | 遺留資源在 **2 個 Reconciler 週期（10 分鐘）內**被偵測並清除；重複 `destroy` 無錯且無 404；`cleanup_status` 在清理完成前不得為成功。<br>**注入子項是 X-04 唯一的驗證方式**：注入到超過單節點 50% slot → 該節點必須 drain；再注入到全池 25%（下限 2）→ 必須暫停派送；清掉後連續 2 輪才恢復。**不注入就沒有任何測項會執行到 drain 與暫停這兩條路徑**，等於門檻定了但沒人驗過它會不會動 |
| **T8** | **設定與供應鏈宣告式稽核**：閘門 A 節點准入探針輸出實測值；`runtime-image` 流水線斷言 ＋ 查詢 GHCR 上的 SBOM 與掃描 attestation | 1（映像）／2（節點） | C-01、P-01、P-03～P-05、I-01～I-04、I-06 | 探針對每一項輸出 `pass`／`fail`，**`unknown` 視同 fail**（fail-closed）。**不以滲透測試代替**（`02:SEC-009` 明文） |
| **T9** | **Provider 契約測試**：RUN-009 既有套件對真實 SelfHostedProvider 跑一次（目前只對 Fake 與 dev Docker 跑）；**另加兩項斷言**：①**I-05**——同一 Image 更新後，歷史 Run 記錄的 `runtime_version` 不變；②**A1-e**——送一個 `egress.allow` 與節點已渲染清單不符的 `RunRequest`，必須以 `capability_mismatch` 422 拒絕，而不是接受後讓它逾時 | 2 | 生命週期一致性、X-01、**I-05** | 與 Fake Provider 同一套斷言全綠，加上上述兩項。<br>**I-05 從 T8 移到這裡**：「Image 更新不改變歷史 Run 的 Runtime Version」是**跨時間的行為**，宣告式探針對單一時點的節點狀態拍照，看不到它 |
| **T10** | **P-02 常駐探針**：自沙箱內嘗試連核心資料庫與內部控制服務 | 2 | **P-02** | 節點入池前跑一次**且入池後常駐**（威脅模型 §5.3 要求的是常駐探針，非一次性測試）：連線嘗試必須被阻擋；偵測到成功連線 → 立即終止該 Run 並以最高嚴重度告警（架構回歸訊號）。<br>**從 T8 獨立出來**：其餘 P 區項目是宣告式設定稽核（拍一次照就夠），P-02 明文要求「以探針實際驗證，非僅設定檢查」且必須持續——把它塞在 T8 裡會讓一次性稽核冒充常駐監控 |

**覆蓋核對（45 項）**：

| 區 | 項數 | 覆蓋 |
| --- | --- | --- |
| C 計算隔離 16 | C-01（T8）、C-02～C-09（T1，C-08／C-09 另由 T2 加壓）、C-10～C-15（T3）、C-16（T1＋T7） | 16 |
| P 節點與拓撲 5 | P-01（T1＋T8）、**P-02（T10）**、P-03～P-05（T8） | 5 |
| N 網路隔離 7 | N-01～N-07（T5 的八個子項） | 7 |
| D 資料與憑證 7 | D-01～D-06（T6）、D-07（T7） | 7 |
| I 執行映像 6 | I-01～I-04・I-06（T8）、**I-05（T9）** | 6 |
| X 清理與遺留 4 | X-01（T7＋T9）、X-02～X-04（T7，含注入子項） | 4 |
| **合計** | | **45** |

**補記（2026-08-26）——基線增為 46 項，本表與 §3 的「45」是定案當日的數字。**

`a7e1699` 於 2026-08-25 在威脅模型 §4.3 新增 **N-08**（`ip6` 鏈維持 `policy drop` 且不得渲染 accept 規則），理由是 `infra/egress/allowlist.yaml` 的 `pinned_ip` 全部是 IPv4，若 `ip6` 鏈留在 `policy accept`，一次 AAAA 解析就能整條繞過 N-01。**那次沒有同時改本表**，於是 N 區仍寫 7 項、合計仍寫 45，而 N-08 沒有任何測項指到它——**一個存在於基線、卻不會被評到的檢查**，而且 §3 的判準是「45 項全數 pass」，湊滿 45 不需要碰它。

- **測項歸屬**：N-08 由 **T5** 承接，新增子項 **T5-9 IPv6 旁路**——自沙箱對允許清單上的目的地做 AAAA 查詢並嘗試以 IPv6 連線，以及對清單外的目的地做同樣的事，**兩者都必須 DROP**；另須觀察 `ip6` 鏈的 `policy drop` 與零 accept 規則，並在 nftables 記錄中留下 drop 計數（與 N-06 同一要求）。Suite 2，與 T5 其餘子項同批。
- **§3 判準的現行讀法**：原文不改寫。它的語意是「基線全數 `pass`、0 項 `unknown`」，而基線自 2026-08-25 起是 **46 項**，因此現在要數的是 46。
- **這一格為什麼補在這裡而不是新開 ADR**：本 ADR 的決策沒有被推翻。基線在它之後多了一項，而覆蓋核對是對基線的對帳表，不是決策本身；漏掉的是對帳，補的也是對帳。

**補記（2026-08-27）——兩個測項照字面執行會得到沒有用的結果，第三件是一個沒有人寫下來的節點前提。**

本節的 10 個測項在 2026-08-27 第一次有兩項被真的執行（[T5 網路實驗室](../plans/mvp/m4/sec-009-acceptance/2026-08-27-nested-dev-container-t5/)與[閘門 A 探針](../../tools/sec009/t8-node-probe.py)），而**執行它們才看得到的三件事**列在 [`05` R-17](../plans/05-pending-rulings.md)，此處只記事實與歸屬：

- **`C-01` 的執行期半邊，閘門 A 在結構上看不到**（它是關於兩個並行 Run 的敘述，而閘門 A 拍的是一台閒置節點）。照 §3 的 fail-closed 讀法，**一台完全正確的節點也會 exit 2**，而永遠紅的閘門會被關掉。→ R-17a。
- **`T5-5` 的節點 loopback 子項留不下 nftables 紀錄**：封包不會離開沙箱的 namespace，加了路由、`route_localnet=1` 與 `prerouting` raw 計數規則之後**計數仍是 0**。T5 的判準「沒有記錄 ＝ N-06 未成立」照字面會讓它永遠 `unknown`。→ R-17b。
- **Q2 條件 2 的東西向阻擋有一個沒有寫下來的前提**：`iifname X oifname X drop` 與 `--icc=false` **兩者都依賴 `br_netfilter` 已載入且 `bridge-nf-call-iptables=1`**。同一個 bridge、同一個網段上的兩個沙箱走的是二層，`ip` 的 forward hook 看不到那些 frame。**在 stock `ubuntu-latest` 核心上實測：模組未載入，東西向連線成功**——兩個手段一起、安靜地失效。**節點的 IaC 負責這個前提，閘門 A 必須把它列為一項檢查**，因為 `infra/egress/rendered/` 裡沒有任何東西能自己保證它。

**這三格為什麼補在這裡而不是新開 ADR**：本 ADR 的決策一條都沒有被推翻——Q2 條件 2 要的東西向阻擋、§2 的覆蓋表、§3 的 fail-closed，全部維持。變的是**它們要怎麼被判定**，而那是對帳不是決策；形式比照本節 2026-08-26 的 N-08 補記。**判定表本身的修改要等 R-17 裁定。**

## 3. 整體通過判準

- **45 項全數 `pass`，0 項 `unknown`。**
- 任一項 `fail` 或 `unknown` → **`SelfHostedProvider` 不得開放外部使用者提交 Skill 執行**（ADR-015 定案紀錄的語意界線，本 ADR 不放寬）。
- 部分通過**不得**以「其餘項目風險低」為由放行。這一條沒有例外流程——若某項在可預見期間內結構性做不到，正確動作是**修改基線並記錄為新的 ADR**，不是在驗收表上放水。

## 4. 誰跑、什麼時候跑

| 時機 | 跑什麼 | 誰 |
| --- | --- | --- |
| **每次節點加入池之前** | Suite 2 全部 | 執行節點 IaC 的人（部署批負責人） |
| **每次 `services/sandbox/**`、`infra/images/**` 變更** | Suite 1（T4 除外） | CI（ADR-019 job 5 之後新增一個 job） |
| **Runtime Image 或 gVisor 版本變更** | Suite 1 全部（含 T4） | CI ＋ 人工確認 T4 的逐筆歸因 |
| **首次全套（上線授權）** | Suite 1 ＋ Suite 2 | 部署批負責人執行，**產品負責人見證並簽署**——這是「可以開放外部使用者」的那個簽名 |
| **每次 P-03 的 7 天重建** | Suite 2 的 T8（閘門 A 探針） | 自動，隨節點開機執行並上報 |

## 5. 證據放哪

**`plans/mvp/m4/sec-009-acceptance/`**，每次執行一個 `YYYY-MM-DD-<node-id>/` 子目錄。**放 M4 不放 M2**：M2 已完結，往一個結案的里程碑目錄裡持續寫入新證據會讓那份對帳失去「某一時點的帳」的意義；而這批驗收的實際歸屬是上線前的封測準備（M4）。

**進 repo 的（小、要被 review、要能追溯）**：

| 檔案 | 內容 |
| --- | --- |
| `README.md` | **45 列的判定表**（檢查 ID／測項／pass·fail／證據連結）＋執行環境摘要＋簽署人 |
| `versions.txt` | `runsc --version`、Docker／containerd 版本、節點 IaC commit SHA、Runtime Image digest、GHCR attestation 摘要 |

**留在 CI artifact／節點側，由判定表逐列附連結的（大、機器產生、沒人會逐行讀）**：`probe.json`（閘門 A 探針原始輸出）、`T1..T10/` 各測項的 stdout／stderr、`nftables-counters.txt`（測試前後的規則與計數器快照——T5 的證據核心：擋下來要看得到計數）。

保存期 **≥ 1 年**（比 Trace 的 90 天長：這是上線授權的證據，不是運維資料）。CI artifact 的預設保留期短於一年，故該類證據須在判定表簽署時一併封存至物件儲存並在 `README.md` 記下位址。

---

## 影響

### 正面

- **SEC-002 的六項無值語句全部有值**，45 項基線首次可被自動化判定；`02:SEC-002` 的「未定值前不得記為通過」條款解除。
- **Q1～Q3 有答案**，`02:SEC-002` 的勾選前提成立；SBX-007 的「允許清單管理流程仍是 ADR-015 待決策」不再是阻擋理由（剩下的是實作，不是決策）。
- **SEC-009 從一句話變成 9 個測項、45 項覆蓋核對與一份證據清單**，部署批拿到即可執行。
- **不新增任何元件**：沒有 Squid、沒有 Envoy、沒有叢集排程器。生產形態與 dev 形態同語意，差別只在強制手段（路由 → nftables）。
- **發現並具名了一個既有的 dev 缺口**（同一出口網路上的 sandbox 可互通 ＝ 不需逃逸的跨 Run 橫向路徑），生產形態把它關掉並列為驗收測項。
- **縮小 ADR-019 待決策 3**：gVisor 不需巢狀虛擬化，多數隔離測項可在 GitHub-hosted runner 執行（待部署批實測確認）。

### 成本與限制

| 項目 | 說明 |
| --- | --- |
| **同節點多 Run 的橫向風險被接受，不是被解決** | Q2 決策的明示殘餘風險。緩解是 7 天重建 ＋ 一鍵停用，不是隔離 |
| **允許清單以 IP:port 表達** | 目的地換 IP 就要重建節點。對平台自有服務正確，對第三方不正確——重評條件已寫死 |
| **egress 記錄沒有 L7 粒度** | 只有 IP:port／協定／位元組。允許清單只有一項時 IP 即身分；長出第二項就不是了 |
| **I-03／I-04 的落點已定，但流水線尚未接上** | GHCR 已定案，SBOM 與掃描 attestation 有了隨 image 保存的地方；**發佈流水線接 GHCR push ＋ attestation 是實作工作（SBX-011）**。在它完成前 `scanned_at` 仍是人工維護的日期，**這是 SBX-002 維持不勾的唯一剩餘原因**——但它從「結構性不可達」降級為「一件待做的工作」 |
| **X-03／X-04 需要新指標才量得到** | 「連續 2 輪同一筆」與分項失敗計數器都不在現有 metrics 裡（SBX-012）。在它完成前 `alerts.yml` 的對應規則是過渡形式，只說得出「這段時間銷毀過／失敗過」，說不出「同一個東西還在」 |
| **每日 cron 依賴 GitHub Actions 排程** | Actions 的 `schedule` 在高負載時會延遲甚至跳過。24 h SLA 自 issue 開出起算，因此延遲會直接延後 SLA 起點——這是本 ADR 已知且接受的鬆弛，替代方案（自架排程器）為了這一件事不值得 |
| **24 小時 CVE SLA 的實質是「換版或停用」** | 單人團隊不保證 24 小時內換得完版；保證的是 24 小時內不會有一個帶已知逃逸漏洞的池在服務外部使用者 |
| **X-04 的比例門檻在極小池上很敏感** | 封測 2 slot/節點下，單節點門檻 50% ＝ 1 筆。這是刻意的（N+1 撐得住），但節點數降到 1 時該門檻會變成「一次洩漏即全停」——池只剩一台時應直接視為降級狀態並告警 |
| **T2 是煙霧級 fuzz** | 不宣稱等同研究級 syscall fuzzing。真正的 gVisor fuzzing 是上游的責任，本平台的對應控制是 P-04 的版本跟隨 SLA |
| **新增一個 CI job** | 允許清單變更必須連帶更新威脅模型的 path-filter 比對。成本極小，但 ADR-019 的「path filter 是手寫的」缺點同樣適用 |

## 驗證方式

- 第三部分即本 ADR 的驗證方式：**45 項全數通過且 0 項 unknown，才算本 ADR 的決策被證實**。在那之前，本 ADR 與 ADR-015 一樣，表示的是方向與判準，不是「隔離強度已被驗證」。
- Q3 的核心斷言（「除了閘道以外沒有路由」）由 T5 的六個子項驗證，其中 T5 的**反向驗證**（到閘道必須成功）是防止「測到的其實是全斷網」的必要對照。
- 六項門檻值的**校準義務**：P-03、X-02、X-03／X-04 是依封測規模推的假設值，與 ADR-015 的容量模型同性質——**首次真實部署後須以實測回頭校準**，特別是 X-03／X-04 的誤報率（若封測期間告警全部是誤報，門檻定得太緊）。

## 待決策

- **威脅模型 Q4**：執行中 egress 阻擋事件的終止門檻（幾次違規終止 Run）。本 ADR 提供了計數的來源（nftables drop 記錄關聯 `run_id`），**但不定值**——它需要一次真實部署的誤報率資料才定得出來，現在定就是猜。歸屬：部署批第一次校準。
- ~~**威脅模型 Q17**：安全事件嚴重度分級與值班責任歸屬。本 ADR 把 X-03／X-04-6b 與 P-02 探針標為「最高嚴重度」作為 Q17 的輸入，但分級表本身仍未定案。歸屬：SEC-010。~~ **→ 分級表已定案（2026-08-16，與本 ADR 同日）**：見 [`02:SEC-010`](../plans/02-specifications-and-acceptance-criteria.md) 的「事件嚴重度分級與回應」小節（同日經負責人核可生效），[P1 停止派送 runbook](../runbooks/p1-dispatch-halt.md) 以它為對應決策。**未定案的只剩「值班責任歸屬」那一半**——該節逐字寫著「值班輪替的討論待團隊有第二人時重開」，而今天團隊仍是一個人。
- ~~**Container registry**（ADR-019 待決策 1）~~ → **已定案（2026-08-16）**：GHCR，見上方「補充決策」。剩下的是實作（SBX-011）。
- **節點池預熱策略**（自 ADR-015 承接，未回答）：本 ADR 決定了節點怎麼建、多久重建，但沒有決定「是否預先起好 sandbox 以省掉冷啟的那一分鐘」。與成本模型的「單 Run 佔用 6 分鐘」直接相關，屬部署批的校準範圍。
- **`sandboxd` 自身的更新機制**：節點重建會換掉它，但緊急修補（例如 sandboxd 自己的漏洞）是否允許不重建節點就滾動更新，未定。傾向「不允許，一律走重建」以維持節點無狀態的性質，但未做成決策。

---

## 審查修正紀錄（2026-08-16）

獨立審查對本 ADR 做對抗性審視，**三個選型結論（Q1／Q2／Q3）全部維持，狀態維持 `Accepted`**。以下異動全部屬量測點補強、回填、一致性修正與限制條件收緊；需要新增決策內容的三項（A1-a、A1-b、Container registry）以增補段落完成，**未原地改寫任何既有決策**。

| 類別 | 異動 |
| --- | --- |
| **新增決策** | Container registry 定案 **GHCR**（回答 ADR-019 待決策 1），連帶把 I-03／I-04 從「結構性不可達」降級為實作工作，並成為 SEC-009 的前置條件。<br>**A1-a** 節點層的強制形式與沙箱層不同（FQDN 比對，不做 IP 釘選），`pinned_ip` 改為 tier 條件必填，重評條件只約束 `tier: sandbox`。<br>**A1-b** 升為 Q2 強制條件 6：沙箱層目的地不得是控制平面位址，LiteLLM 須有沙箱面專屬位址（成本 $10.38，由既有的 Egress Proxy 預算行承載，總額不變） |
| **量測點補強** | **A2-a** P-04 增第二量測點（每日 cron 比對上游 releases／advisories），24 h SLA 起算點改為「cron 開出 issue 的時刻」。<br>**A1-c** Q3 的重評條件與威脅模型連動改為 CI 斷言。<br>**A1-e** sandboxd 於 `accept()` 比對 `egress.allow` 與節點已渲染清單，不符即 `capability_mismatch`。<br>**B-3／F-6** X-03／X-04 量測改為 in-flight orphan 表與分項失敗計數器，並標明需新指標（SBX-012） |
| **對齊實作** | **B-2／F-5** X-02 的年齡寬限改述為「只給平台不認得的 sandbox 的 5 分鐘派工窗口，已辨識終態者立即清」，對齊 `run.orphanGrace`／`isOrphan` 的既有語意；刪去「重用 ObjectGrant TTL、不新增常數」的誤述 |
| **限制收緊** | **B-1** 豁免複審日錨定改 `first_exempted_at`＋90 天（原以掃描日錨定會被每次重掃無限後推）。<br>**B-5** 任一節點 drain 期間暫停 P-03 例行滾動重建。<br>**C-2** Q1 標明有效範圍為封測 2 節點，早期成長 5 節點即越線必須重開評估。<br>**6b** 物件短效授權移出「撤銷失敗」類，改述為窗口上界 ＝ 簽發 TTL 的明示殘餘風險 |
| **驗收覆蓋補洞** | **C-3／D-1** P-02 獨立為 **T10 常駐探針**；新增 **T5-7**（閘道 `pinned_ip` 的非允許 port）與 **T5-8**（自外部掃節點 port）；T7 增「人工注入假遺留資源」子項——**否則 X-04 的 drain 與暫停路徑沒有任何測項會執行到**；I-05 改由 T9 契約測試承接（跨時間行為，宣告式探針看不到）；45 項覆蓋核對表重算 |
| **措辭** | **A1-d** 強制點改述為「主機側 `forward`／`DOCKER-USER` 鏈或 Run netns 內」，避免部署批照 `output` 字面實作成容器內規則（那能被逃逸後改掉）。**D-3** 證據路徑改 `plans/mvp/m4/sec-009-acceptance/`，判定表與 `versions.txt` 進 repo、原始輸出留 CI artifact 並附連結 |
| **隨本次建立的檔案** | [`infra/egress/allowlist.yaml`](../../infra/egress/allowlist.yaml)、[`infra/nodes/gvisor-baseline.txt`](../../infra/nodes/gvisor-baseline.txt)、[`.github/workflows/gvisor-baseline.yml`](../../.github/workflows/gvisor-baseline.yml)、[`.github/workflows/egress-allowlist.yml`](../../.github/workflows/egress-allowlist.yml) ＋ [`check_egress_allowlist.py`](../../tools/ci/check_egress_allowlist.py) |
