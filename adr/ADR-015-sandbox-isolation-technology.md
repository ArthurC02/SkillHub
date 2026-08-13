# ADR-015：SelfHostedProvider 採 gVisor 作為隔離基線

- 狀態：**Accepted**（2026-08-14 定案；定案條件逐項狀態見文末「定案紀錄」）
- 日期：2026-08-13（提出）／2026-08-14（定案）
- 決策者：產品負責人、架構規劃

## 背景

ADR-005 已定義最低安全基線（非 root、唯讀檔案系統、資源限制、預設封鎖網路），但隔離技術選型留待本 ADR。工作負載特性：執行從網路取得的不受信任 Skill Script 與 Agent 推理，I/O 輕量、CPU 中等、單次執行短（分鐘級）、無 GPU 需求。

關鍵前提：「執行任意網路取得的程式碼」是本產品的核心功能，不是邊緣情境。隔離強度的預算應高於一般 SaaS。

## 評估選項

### 選項 A：加固容器（runc＋seccomp／capabilities／cgroups）

- 優點：生態最成熟、性能損失最小、維運最熟悉。
- 缺點：所有工作負載共用主機核心，一個核心漏洞即逃逸。對「以執行不受信任程式碼為業」的產品，這是不可接受的單層防禦。

### 選項 B：gVisor（runsc）

- 優點：使用者態核心攔截系統呼叫，主機核心攻擊面大幅縮小；在一般雲端 VM 即可執行（不需裸機或巢狀虛擬化）；與 containerd 整合成熟，沿用容器映像與工具鏈；Google 以此隔離多租戶不受信任負載多年。
- 缺點：系統呼叫密集與 I/O 密集負載有可觀性能損失（本工作負載可接受）；少數 syscall 不相容需驗證目標 Runtime。

### 選項 C：MicroVM（Firecracker／Cloud Hypervisor）

- 優點：硬體虛擬化邊界，隔離最強；啟動百毫秒級。
- 缺點：需要 KVM（裸機或支援巢狀虛擬化的 VM），限縮部署平台選擇；rootfs、網路、裝置模型皆需自管，維運成本最高。

### 選項 D：受管沙箱服務接入 Provider Port

- 優點：隔離與維運責任外包，上線最快。
- 缺點：ADR-005 已決策初期自建以掌握 Trace、政策與產品流程；成本隨用量線性成長且不可控。

## 決策

採選項 B：gVisor 為 MVP 隔離基線。

- 執行節點為獨立 VM 池（專用節點群），與控制平面依 ADR-001 網路與身分隔離；不與一般應用工作負載混排。
- gVisor 之上仍疊加 ADR-005 全部基線（非 root、唯讀根檔案系統、資源限制、無管理 Socket）——gVisor 是多加的一層，不是替代。
- 節點以 IaC 建置、無狀態、定期重建（縮短潛伏攻擊窗口）。
- Egress 全部經專用 Proxy：default-deny、目的地允許清單、DNS 固定解析（防 Rebinding）、記錄目的地與流量不記錄內容。

升級與對沖路徑（依 ADR-004 Provider Port，不改核心）：

- 威脅模型升級或多租戶規模成長 → 評估選項 C（MicroVM）作為高安全等級 Provider。
- 自建維運負擔超出承受 → 以選項 D 的受管服務實作第二個 Provider，逐步遷移。

## 驗證方式

- 逃逸測試：以公開容器逃逸 PoC 集與 syscall fuzzing 驗證邊界（對應 SEC-009、SBX-010）。
- Runtime 相容性：PDM-004 選定的語言 Runtime 在 gVisor 下通過完整 Run 生命週期。
- 資源耗盡與網路外洩測試：fork bomb、磁碟填滿、DNS tunneling、內網掃描皆被阻擋並產生安全事件。
- 契約測試：與 Fake Provider 同一套（RUN-009）。

## 影響

### 正面

- 在一般雲端 VM 即可達到強隔離，不限縮部署平台選擇（與 ADR-014 的平台決策解耦）。
- 沿用容器工具鏈，映像建置、掃描與 SBOM 流程（ADR-005）不變。
- 升級到 MicroVM 或受管服務都在 Provider 邊界內，核心不動。

### 成本與限制

- 性能損失需納入 Run 時間與成本估算（NFR-004 要求量測 Sandbox 建立時間）。
- gVisor 版本需跟隨上游更新，安全修補是持續責任。
- syscall 相容性問題可能在新 Runtime 加入時重現，每次擴充 Runtime 都需重跑相容性驗證。

## 定案紀錄（2026-08-14）

負責人於 2026-08-14 批准 M0 全部產出並指示開工。**選項 B（gVisor 為隔離基線）與「執行節點為獨立 VM 池、不與應用工作負載混排」的邊界維持不變，狀態由 Proposed 轉 Accepted。** [ADR-018](./ADR-018-containerized-core-infrastructure.md) 把控制平面改為容器化自架，**明確把 Sandbox 執行平面排除在容器編排之外**，本 ADR 的平面隔離邊界未被動到。

定案條件逐項狀態：

| 原定案條件 | 狀態 | 依據 |
| --- | --- | --- |
| PDM-003 Runtime 確認 | **已滿足** | Claude Agent SDK（TypeScript）／Node.js 22 LTS；LiteLLM 閘道相容性 **7/7 PASS**、Agent SDK 的 Skill 載入路徑 **6/6 PASS**（正確路徑為 `.claude/skills/`，原假設已證偽）。見 [pdm-003-litellm-spike-report.md](../plans/mvp/m0/pdm-003-litellm-spike-report.md) §4、§10、§11 |
| 成本試算（部署平台的成本面） | **已滿足** | [cost-estimation.md](../plans/mvp/m0/cost-estimation.md) v2（含 §6.2.3 v3 模型成本重估）。Sandbox 節點供應商與型號隨 ADR-018 的 Hetzner 主推／DO 退路 |
| PDM-004 Runtime 語言與版本 | **未定案**（提案完整，負責人未逐項批示） | 影響的是 Runtime Image 內容（SBX-002），不是隔離技術選擇；不阻擋本 ADR 定案，但**每次擴充 Runtime 都需重跑 gVisor syscall 相容性驗證**（上方「成本與限制」既有要求） |
| 部署平台支援驗證（在選定平台上實跑 runsc） | **未執行 → 轉為實作期驗收前提** | 平台已由 ADR-018 選定（Hetzner Cloud 主推，DO 退路）；實跑驗證併入 M2 Sandbox 工作 |
| 逃逸測試通過（SEC-009、SBX-010） | **未執行 → 轉為實作期驗收關卡** | 這兩項本來就是需求 ID 承接的實作工作，不是文件階段可完成的項目 |

> ⚠️ **定案的語意界線**：本 ADR 轉 Accepted 表示「採用 gVisor 這個方向、後續實作應遵循」，**不表示隔離強度已被驗證**。上方最後兩列是**上線前的硬性關卡**——SEC-009／SBX-010 未通過不得開放外部使用者提交 Skill 執行。

**Hetzner 濫用政策的連帶工作**（隨 ADR-018 選定 Hetzner 而生效）：Sandbox 執行不受信任程式碼，若被用於挖礦、掃描或濫用流量，Hetzner 處置偏向直接停機。除本 ADR 既有的 default-deny Egress Proxy 外，另需**事前與供應商溝通用途**、建立濫用偵測與自動封停 Run 的流程（對應 SEC 需求）。見 [cost-estimation.md](../plans/mvp/m0/cost-estimation.md) §7.1。

## 待決策

- ~~節點編排：Kubernetes（RuntimeClass=runsc）或純 VM＋輕量 Agent。~~ → **已回填（2026-08-14）**：採**純 VM 池＋輕量 Agent，Sandbox 節點不進 Kubernetes**。理由是把 Sandbox 放進與應用共用的編排層，會讓 ADR-001 的平面隔離退化為命名空間隔離。此為 [ADR-018](./ADR-018-containerized-core-infrastructure.md) 明列的容器化例外項；見 [cost-estimation.md](../plans/mvp/m0/cost-estimation.md) §5.1、§7.3。
- Egress Proxy 具體實作與允許清單管理流程。→ **部分回填（2026-08-14）**：MVP 允許清單維持三項目的地——LiteLLM 閘道、物件儲存短效授權端點、Trace ingestion 端點；首批三個 Skill 類別（`documents`／`writing`／`data`）皆為「上傳檔案進、產出檔案出」，**不需為任何類別開放額外目的地**。見 [pdm-proposals.md](../plans/mvp/m0/pdm-proposals.md) §1、§5.2。**具體實作與清單變更流程仍未決，移交 M2（SBX-007）。**
- ~~容量池大小、預熱策略與單 Run 成本目標（PDM 相關決策後校準）。~~ → **第一版數字已回填（2026-08-14，依 [cost-estimation.md](../plans/mvp/m0/cost-estimation.md) v2 §2.1／§2.2／§4）**：
  - slot 規格 **2 vCPU / 4 GiB**（4 GiB/slot 使記憶體成為約束，淘汰所有 2 GiB/vCPU 的運算最佳化機型）；單 Run 佔用 **6 分鐘**＝4 分執行＋1 分 provisioning 與 gVisor 冷啟＋1 分清理（gVisor 相對 runc 約 +20% wall time 已內含）。
  - 容量池：尖峰併發 slot ＝ `每日 Run × 6min ÷ 8h × 2`，節點冗餘一律 **N+1**。封測情境（50 Run/日）＝ 2 slot → **2 台 4 vCPU/16 GiB**；早期成長（500 Run/日）＝ 13 slot → **5 台 8 vCPU/32 GiB**。
  - 單 Run 平台成本目標：**$0.19（封測）／$0.064（早期成長）**，以 E1 Hetzner 計。**Sandbox 池佔平台總成本 83%**，是唯一值得優化的一項；優化手段是縮短單 Run 佔用時間與控制 Run 量，不是換編排技術。
  - **預熱策略仍未決**，移交 M2。
  - ⚠️ **這些是假設值不是量測值**。單 Run 佔用時間是敏感度最高的單一變數——翻倍即 Sandbox 池 +60%（早期成長 5 → 8 台）。NFR-004 已要求量測 Sandbox 建立時間，首次真實 Run 後須回頭校準。
