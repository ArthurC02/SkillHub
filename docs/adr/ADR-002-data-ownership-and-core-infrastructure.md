# ADR-002：資料所有權與核心基礎設施

- 狀態：Accepted
- 相關：[ADR-001](./ADR-001-system-context-planes-and-deployment-path.md)（系統情境、平面與部署路徑）、[ADR-003](./ADR-003-run-orchestration-and-async-workflows.md)（Run 編排與非同步工作流程）、[ADR-004](./ADR-004-sandbox-isolation-and-execution-security.md)（Sandbox 隔離與執行安全）、[ADR-005](./ADR-005-model-gateway-and-observability.md)（模型閘道與可觀測性）、[ADR-006](./ADR-006-identity-workspace-admission-and-allowances.md)（身分、Workspace、准入與額度）、[ADR-008](./ADR-008-intent-search.md)（意圖搜尋）、[ADR-023](./ADR-023-account-purge-and-credit.md)（帳號清除與 Credit）

## 背景

Skill Hub 同時持有多種形狀不同的資料：關聯式領域資料、不可變的 Skill 套件、使用者 Dataset、大量且持續寫入的 Run Trace、可重建的搜尋投影，以及必須與一般資料隔離的 Secrets。全部放進同一個資料庫會在容量、查詢型態與生命週期政策上互相衝突；但團隊規模小，維運能力是最稀缺的資源，過早導入多種資料產品會直接吃掉這個資源。核心基礎設施因此需要回答兩件事：每一類資料的事實來源與擁有者是誰，以及承載這些資料的產品要「買」還是「自架」、架在哪裡。

## 決策

### 決策 1：資料依特性分派事實來源與擁有者，派生資料一律可重建

| 資料 | 事實來源 | 主要擁有者 | 特性 |
| --- | --- | --- | --- |
| User、Workspace、Skill metadata、Run 狀態 | 關聯式資料庫 | 對應領域模組 | 交易、一致性、查詢 |
| Skill Package、Dataset、Artifact | 物件儲存 | 對應領域模組 | 大型 Binary、不可變、生命週期政策 |
| Skill Search Document | 資料庫搜尋投影（FTS＋pgvector） | Catalog | 可重建、讀取最佳化 |
| Run Trace | Postgres 分割表（大型 Payload 進物件儲存） | Run／Trace | 追加寫入、大量、依時間分割 |
| API Key、Provider 憑證 | Secrets Store | 對應領域模組 | 加密、短效授權、輪替 |
| Usage／Cost 事件 | 關聯式資料庫 | 對應領域模組 | 不可變計量事件 |

原始 Skill Package、其內容雜湊與不可變 Skill Version 是 Skill 的事實來源；搜尋摘要、Embedding、分類與排序特徵都是可重建投影，遺失後可從事實來源重算。Run 的定義與狀態由控制平面保存，Provider 回報的狀態不是事實來源。Trace 與 Artifact 不得反向修改歷史 Test Case 或 Skill Version。Secrets Store 只保存密鑰材料本身，領域資料庫只保存 Secret Reference 與必要 metadata；平台自身憑證存放於部署平台的 Secrets Manager，使用者憑證（後 MVP，隨 MCP）另以 KMS envelope encryption 設計。

### 決策 2：每次 Run 固定引用一組不可變快照

一次 Run 必須固定引用：`skill_version_id`、Test Case 快照 ID、Agent／模型與版本、Runtime Profile、Provider 與 Capability Snapshot、權限與網路政策 Snapshot、Dataset 內容雜湊。事後修改 Skill、Test Case 或 Provider 設定，不得改變任何一筆歷史 Run 當時引用的內容——這是 Run 可重現性與可稽核性的唯一保證來源。

### 決策 3：物件存取一律短效、範圍限定於單一物件或單一 Run

Sandbox 取得的物件存取權限一律短效，且限定在單一物件或單一 Run 範圍；執行平面不得取得列舉整個 Bucket 的權限。使用者上傳的 Artifact 先進隔離區，通過大小、類型與安全檢查後才開放下載；下載一律使用短效授權連結，不對外公開永久物件 URL。

### 決策 4：使用者資料預設要求 Workspace 範圍

所有使用者擁有的資料至少包含 `workspace_id`，或能從父層資料無歧義地導出。查詢層預設要求 Workspace 範圍，不接受由 UI 單獨保證隔離、也不信任呼叫端傳入的 `workspace_id`。多租戶政策、准入與額度的完整規則見 [ADR-006](./ADR-006-identity-workspace-admission-and-allowances.md)；本決策只定資料層的預設邊界。

### 決策 5：刪除使用者輸入只清內容，不抹除可追溯性

使用者刪除 Dataset 或 Run 輸入後，歷史 Run 保留內容雜湊、metadata 與 Trace 引用，並標示「輸入已刪除」；Run 維持可追溯（能證明當時用了什麼）但不再保證可重現（無法重新執行）。UI 與評估報告不得在輸入已刪除時暗示仍可重跑或比較。刪除跨越物件儲存、索引與 Trace 時，一律使用可追蹤的非同步工作流程，不宣稱瞬間完成；牽涉整個 Workspace 的不可逆清除另見 [ADR-023](./ADR-023-account-purge-and-credit.md)。

### 決策 6：一致性策略分三層

單一模組內的重要狀態變更使用資料庫交易；資料變更與對外事件使用 Transactional Outbox 同交易寫入，Outbox 本身即佇列，消除「寫庫成功、發訊失敗」這一類故障；搜尋索引與其他讀取投影接受最終一致性，UI 在必要處顯示處理中狀態。

### 決策 7：核心基礎設施以 PostgreSQL 為中心，只挑最少必要的產品

MVP 規模下由單一 PostgreSQL 同時承載交易資料、全文與向量檢索（[ADR-008](./ADR-008-intent-search.md)）、佇列（River，同庫同交易）與 Trace 分割表，理由是每多一個資料產品就多一整套備份、監控、升級與失敗模式，而團隊維運能力是最稀缺的資源。所有元件只透過標準介面（SQL、S3 API、OIDC、OpenTelemetry）存取，供應商可替換。LiteLLM 共用同一個 PostgreSQL 實例但使用獨立邏輯 database，理由是 migration 不得互相波及。Trace 分割表依時間切分並定期 `DROP PARTITION` 釋放容量，保存天數由 Policy 設定而非程式邏輯決定。Postgres 佇列的吞吐上限約在每秒千級任務內；超過此上限、需要多服務獨立消費、或需要重播歷史事件時，才評估遷移至受管 Queue——因 Consumer 已要求冪等，遷移不改變呼叫語意。是否新增獨立的非同步工作流程引擎，取決於補償邏輯或人工介入流程的複雜度，見 [ADR-003](./ADR-003-run-orchestration-and-async-workflows.md)。

### 決策 8：控制平面與資料層容器化，自架於 Hetzner Cloud 單節點

控制平面服務（Go API、Worker、Python LLM 服務）與 PostgreSQL 皆以容器跑在自有節點（`infra/compose/docker-compose.yml`），與應用同節點、同內網，消除跨供應商延遲風險，且不受外部供應商計費模型變動影響。PostgreSQL 備份採 pgBackRest 或 WAL-G 持續 WAL 歸檔至物件儲存；備份還原演練（不是備份設定，是還原）是單實例架構可被接受的前提，未演練過的備份等同沒有備份。單實例沒有自動 failover，節點或容器故障即代表全平台停機至人工還原，此風險明示接受、不得默認為與受管服務同等可靠。若濫用政策風險不可接受，DigitalOcean 是完整的退出路徑（單一供應商、每秒計費對短命節點有利），代價是月費顯著提高。

### 決策 9：物件儲存是容器化的例外，採供應商受管 S3 相容服務

物件儲存不隨控制平面容器化自架，改用供應商的 S3 相容受管服務；理由是自架物件儲存（以節點內含 Volume 承載）比受管服務貴 10–30 倍，且無跨節點耐久性。應用程式碼只透過標準介面（S3 API）存取，不直接依賴特定供應商，後端仍可替換。短效 presigned URL 是唯一的存取方式，不對外公開永久物件 URL。

### 決策 10：單節點吃緊時的升遷觸發條件

以下任一成立即評估切換到受管 Kubernetes（取得 CNPG 自動 failover）；全部未成立就留在單節點，不預先建設編排層：

1. PostgreSQL 單實例的計畫外停機已影響 Run 完成率。
2. 控制平面節點數超過 4，手動編排的部署、滾動更新與設定漂移成本超過學習與升級成本。
3. 服務拆分觸發條件已成立（見 [ADR-001](./ADR-001-system-context-planes-and-deployment-path.md)）。
4. 團隊具備至少一名可負責 Kubernetes 版本升級的成員——這是必要條件而非加分項；前三項成立但此項不成立時，正確動作是先做 PostgreSQL 備援，不是上 Kubernetes。

切換時應重新試算成本，不沿用切換前的數字。
### 決策 11：控制平面的編排層是 docker compose，設定由 cloud-init 產生，機密只以檔案手動注入

編排層取 docker compose，不預先建設 k3s 或 Kubernetes；升遷的觸發條件見決策 10。生產控制平面的組合定義是 `infra/compose/control-plane.yml`（`infra/compose/docker-compose.yml` 是本機開發用），節點設定由 cloud-init 產生、`tools/deploy/render.py` 渲染，節點上只 checkout 部署需要的路徑，不放應用程式原始碼。不含秘密的設定（角色、commit、網域、釘住 digest 的映像）進 `/etc/skillhub/release.env`；秘密一律是 `/etc/skillhub/secrets/` 底下的檔案，目錄 700、檔案 600、由人手動放置，不進 git、不進 compose 檔、不進映像。操作步驟見 [控制平面 Runbook](../runbooks/control-plane.md)。

## 影響

### 正面

- 大型物件、搜尋、Secrets 與交易資料各自使用合適的儲存模型，但共用最少的產品數量。
- 可重建投影降低對任一儲存或搜尋供應商的綁定。
- 不可變快照與 Trace 引用支援歷史可追溯，同時容許使用者刪除輸入內容。
- 控制平面與資料層同節點消除跨供應商延遲風險，不受外部計費模型變動影響。
- 應用程式碼不因基礎設施跑在哪裡而改變——標準介面的抽換契約沒有被打破。

### 成本與限制

- 需要處理跨儲存（資料庫／物件儲存／搜尋索引）的刪除、重建與一致性狀態，刪除不是瞬間完成。
- 資料庫備份不再等同完整系統備份，且必須定期演練還原，否則備份形同不存在。
- PostgreSQL 單實例是目前架構下的單一故障域：無自動 failover，故障即全平台停機至人工還原；維運工時因此高於受管方案，不靠自架省錢回本，理由是資料主權與延遲風險控制而非成本。
- 必須另外定義並落成 Policy 設定：Skill／Dataset／Trace／Artifact／Secrets／Usage 的保存與到期規則，不得停留在原則層級。
