# ADR-015：SelfHostedProvider 採 gVisor 作為隔離基線

- 狀態：Proposed（定案條件：PDM-003／004 Runtime 確認、部署平台支援驗證、逃逸測試通過）
- 日期：2026-08-13
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

## 待決策

- 節點編排：Kubernetes（RuntimeClass=runsc）或純 VM＋輕量 Agent。
- Egress Proxy 具體實作與允許清單管理流程。
- 容量池大小、預熱策略與單 Run 成本目標（PDM 相關決策後校準）。
