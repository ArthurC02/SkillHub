# DDD／Clean Code 收斂：Coding Agent 執行規格

本檔給要動 `apps/platform/internal/` 領域程式碼的 Coding Agent，講的是**領域規則該住在哪裡**：規則的定義在 Go，原子性機制留在 SQL，使用者看得到的句子在 handler。日常的跨 context 判斷看 [platform-ddd-practices.md](platform-ddd-practices.md)。

四段各自回答一個問題：**§4** 現在住在哪裡、**§5** 哪些看起來該做而量測說不做、**§6** 還缺什麼、**§7～§10** 怎麼驗與什麼時候停。§1～§3 是動手前要先過的約束、判準與形狀。

動手前先讀根目錄 [`AGENTS.md`](../../AGENTS.md) 與 [`apps/platform/internal/AGENTS.md`](../../apps/platform/internal/AGENTS.md)，以及目標套件的 `doc.go`。

---

## 0 執行協定

1. **一次做一件事**，§7 的驗證全綠才開始下一件。
2. **本檔不放靜態清單，只放 DISCOVER 指令。** 指令輸出與本檔描述不符時，**以輸出為準**，停下並回報差異，不要照記憶動手。
3. **裸 `grep` 在部分開發機會被外掛改寫。** 一律用 `git grep` 或 `awk`。用裸 `grep` 得到的數字不可採信。
4. **PROVE 沒有紅過就不算修好。** 沒有紅，回報裡就不要出現「修好了」。
5. 每個任務結束時工作樹必須：`go build ./...` = 0、`gofmt -l` 無輸出、該套件測試綠、`comment-lint` = 0。

---

## 1 絕對約束

違反任一條 = 立即回退該任務並回報，不要嘗試修補。

| # | 約束 |
| --- | --- |
| C1 | **不得刪除或放寬任何 DB constraint、trigger、unique、外鍵。** 本工作不產生任何 `DROP CONSTRAINT`／`DROP TRIGGER`／放寬 `CHECK` 的 migration。新增可以，減少不行。 |
| C2 | **原子性機制留在 SQL**：`WHERE … AND status = @from_status`、`enforce_immutable()`、`revision = @expected_revision`。這些不是「SQL 擁有領域概念」，是併發保證，Go 做不到。 |
| C3 | **規則的定義寫在 Go**，且必須有**不連資料庫**就能跑的測試。 |
| C4 | **SQL 不得擁有領域概念。** 判準 J1。 |
| C5 | 高衝突區只由主 Agent 序列化：`contracts/`、`db/migrations/`、`db/queries/`、generated 目錄、`go.sum`／`package-lock.json`／`uv.lock`、`Taskfile.yml`、`.github/workflows/`。 |
| C6 | codegen（`task gen:sql`／`task gen:openapi`）只由主 Agent 跑；提交前 `task gen:check`。generated 檔禁止手改。 |
| C7 | **不寫註解。** 唯一例外：艱難演算法，一個區塊最多 3 行，說明它怎麼運作。施工日誌、決策說明、需求編號、日期一律禁止。 |
| C8 | 每個宣稱修好的東西都要有一次紅（§8；[automation.md](automation.md) 開發自動化第 9 條）。 |

Git 紀律（禁止 `git stash`、不對他人修改用 `git reset`／`git clean`／`git checkout -- <path>`、只以明確 pathspec stage）與子代理紀律（預設唯讀、每次派工指定 model、禁用旗艦級、不做 git 寫入）是 repo 級規則，正文在根 [`AGENTS.md`](../../AGENTS.md)〈開發自動化〉第 3 條，本檔不複製。

---

## 2 判準

動手前先過判準。答「否」就不要做，並在回報裡寫出是哪一條擋下的。

**J1 領域概念歸屬**

> 這個領域問題（「這是終態嗎」「這個授權關了嗎」「這個 artifact 是 run 產出嗎」）能不能**在不連資料庫的情況下**回答？

不能 → SQL 擁有它 → 要補 Go 的定義。能 → 已經合格，不要動。

**J2 值物件四問**（要不要把一個裸型別包起來）

有規則？會跟同型別的別的值搞混？有運算？會跨邊界？**四問全否 → 不要包。**

> 第二問要問得比字面更細：**型別只分辨種類，不分辨角色。** 兩個同種類的值（兩個使用者、兩個版本、兩次 Run）包成同一個新型別之後照樣可以互換，編譯器不會出聲。**同種類的混淆要用具名欄位擋，不是具名型別**——實證與逐類後果見 §5.4。

**J3 `require*` 分類**（可機器判別）

- 無參數（`func (s *Service) requireX() error`）＝ **接線檢查**，守的是這個 Service 被組裝好了沒有。
- 吃事實（`func (s *Service) requireX(facts …) error`）＝ **規格**，守的是領域規則，要能不連資料庫測試。

**J4 詞彙要不要型別化**

跨層、會被外部看到（契約 enum、DB 欄位、API 回應）→ 是。只在單一 adapter 內流動 → 否。

**J5 什麼時候不要加 `Parse` 拒絕未知值**

詞彙還在快速增刪時（新值可能先部署到一半），只做常數與對帳，**不要**拒絕未知值——那會讓新值在舊節點上變成硬錯誤。

---

## 3 參考實作：照抄這個形狀

`apps/platform/internal/creator/creation/state.go` 與 `state_test.go` 是所有狀態機任務的範本。

```
type State string                       // 具名字串型別
const ( StateX State = "x" … )          // 型別化常數，值＝契約 enum 的字面值
func AllStates() []State                // 封閉集合的唯一宣告
var successors = map[State][]State{…}   // 轉移表；終態不出現在 key
func ParseState(string) (State, bool)   // 邊界 parse，不合法回 false
func (s State) HasEnded() bool          // 具名謂詞取代散落的 == 比較
func (s State) AwaitsTheModel() bool
func CanTransition(from, to State) bool // 兩端都先 Parse；from == to 一律放行
```

**三條適用於每個狀態機的規則**

1. **`from == to` 必須放行。** 寫入路徑常把同一個狀態原樣寫回，只為推進 revision 或補一筆事件。Postgres 的 `enforce_run_status_transition()` 也是這個語意（`IF OLD.status IS NOT DISTINCT FROM NEW.status THEN RETURN NEW`）。禁止同狀態寫入會直接弄壞現行流程。
2. **兩端都要 Parse。** DB 欄位若沒有 CHECK，讀出來可能是任何字串。未知值一律拒絕寫入（fail closed）。
3. **守衛裝在唯一寫入點。** 先用 DISCOVER 證明只有一個寫入點，再裝。creation 的寫入點是 `service.go` 的 `advance()`。

**測試形狀**（依 [`istqb-test-design`](../../.claude/skills/istqb-test-design/SKILL.md) 技能）

- 測試檔**獨立重述**一份合法集，拿它驗生產表。不要 import 生產表當自己的預言。
- 除黃金表外，每條**性質**各一支測試：終態沒有出口、同狀態寫入放行、某狀態只能由某個來源產生、未宣告的值兩端都拒絕。
- 反假綠：斷言集合大小等於這個狀態機真正的值數（creation 是 `len(AllStates()) != 10` 就 Fatal），否則迴圈跑在空集合上也會綠。

**特徵化先於強制**

沒有書面規格時，轉移表只能從程式碼讀出來。**直接把讀到的東西寫成表，等於把現況裡的 bug 升格成規格。** 順序必須是：先追出每條路徑並寫測試釘住現況 → 再寫表 → 最後才開啟強制。追路徑派唯讀子代理平行做，一個檔一個代理，要求逐行閱讀並附 `檔案:行`。

---

## 4 現況：領域規則現在住在哪裡

| 領域概念 | 定義在 | 不連資料庫的測試 | 機器對帳 |
| --- | --- | --- | --- |
| creation 會話狀態（10 值） | `creator/creation/state.go` | `state_test.go` | `domain-vocabulary` |
| run attempt 的物件授權狀態（4 值） | `trial/execution/grantstate.go` | `grantstate_test.go` | `domain-vocabulary` |
| evaluation 狀態（3 值） | `trial/improvement/status.go` | `status_test.go` | `domain-vocabulary` |
| run 狀態機 | `trial/execution/statemachine.go` | `statemachine_test.go` | `run-status-sql` |
| run 被拒絕的理由詞彙 | `trial/execution/specification.go` 的 `RefusalReasons()` | `specification_test.go` | 同套件的 AST 測試：常數都要在名冊裡、`refused()` 不得寫字面值 |
| 額度拒絕理由詞彙 | `product/entitlements` 的 `AllowanceRefusalReasons` | `quota_test.go` | 測試獨立重述三個字面值 |
| outbox 事件 payload 的形狀 | `foundation/messaging/outbox/events.go` | `payload_test.go` | — |
| 使用者看得到的句子 | 寫出它的 handler，不是領域 sentinel | `messages_test.go`（`trial/design`、`trial/execution`） | 同套件的 AST 測試：`errors.New` 不得帶漢字 |
| Skill 的存取限制是否生效 | `skill/library/access.go` 的 `AccessRestriction`；不能 import owner 的 context 從組裝層收到 `AccessRestricted` 判定，不自己判斷 | `access_test.go` | — |
| 一個 Skill 能不能當參考 | `skill/admission/generate.go` 的 `referenceable` | `generate_test.go` | — |
| 評估的開始與取代、結算、回饋、建議決定、建議已套用 | `trial/improvement/evaluation.go` 的 `Evaluation` aggregate：命令記下事件或拒絕事件，存回只在 `evaluation_store.go`；建議已套用由 `mailbox.go` 消化 Skill 的 `skill.version_added` 後記下（套用結果與既有版本相同時 Skill 沒有變，由 `apply.go` 當下記到那個版本） | `evaluation_test.go` | 事件名稱：`outbox` 套件的 conformance test 對帳 Go 常數 ↔ 最新換上 CHECK 的 migration ↔ 事件目錄 §3 |
| Skill 的建立、加版本、換說明、下架、存取限制、再散布、分類、刪除 | `skill/library/skill_root.go` 的 `SkillRoot` aggregate：命令記下事件或拒絕事件，存回只在 `skill_store.go`；匯入、存新版本、生成、Fork 與套用建議都經過 `AddVersion`，套用建議建成的版本在事件上帶著建成它的評估與建議；`skill/discovery` 只把拒絕理由翻成營運者看得懂的句子 | `skill_root_test.go` | 同上 |
| Run 的轉移、取消、指定 Provider、attempt 的開始／派送／結束、物件授權到期 | `trial/execution/run_root.go` 的 `Run` aggregate：命令記下事件或拒絕事件，存回只在 `run_store.go`；轉移表仍是 `statemachine.go` 的 `successors`，授權狀態仍是 `grantstate.go`；driver 與授權只呼叫命令（排程只組出要釘住的 runtime 快照），清理狀態由 `cleanup.go` 記下 | `run_root_test.go` | 轉移表：`run-status-sql`；事件名稱：同上 |
| creation 還能不能再加訊息、正在等人確認什麼 | `creator/creation/service.go` 的 `Snapshot.hasRoomFor` 與 `PendingAction` 常數（不拒絕未知值，J5） | `snapshot_test.go` | — |

**「機器對帳」欄有兩種東西，不要混為一談：**

- 前四列是 `devctl automation-check` 的檢查器，**Go 與 SQL 分岔時 CI 紅**。`domain-vocabulary` 對帳 Go 常數 ↔ DB `CHECK (… IN (…))` ↔ Postgres enum ↔ 契約 enum，清單是 `tools/devctl/domain_vocabulary.go` 的 `domainVocabularies`，上表只列範本（SQL 沒有 CHECK 的詞彙以 `absent` 寫明）。它同時守覆蓋面：migration 裡每一個 `CHECK (… IN (…))` 詞彙要嘛接進對帳，要嘛在 `unreconciledVocabularies` 寫下為什麼不接（§5.1、§5.6），兩者皆無或理由已經過期都紅；`run-status-sql` 對帳 Go 的 `successors` ↔ migration 0032 的 trigger 轉移列 ↔ 每一處終態 `IN` 清單。
- 其餘各列只有**同套件的測試**，沒有跨 Go／SQL 的對帳——SQL 側要嘛沒有第二份，要嘛只有擋空白字串的 `CHECK`（存取限制，migration 0023）。評估那一列的規則同樣只有同套件的測試；它的事件名稱另由 `outbox` 套件自己的測試對帳。

**閘門順序不在這張表裡，它刻意留在 `create()` 的呼叫序。** 順序決定哪個 reason 先浮出來，而 reason 直接餵 `metrics.RunRefused` 與 `audit.ActionRunRefused`，所以改順序就是改對外行為（§9）。

C1 至今成立：SQL 側沒有任何 constraint、trigger、unique 或外鍵被刪除或放寬。

**認知複雜度上限是 30**（`apps/platform/.golangci.yml` 的 `gocognit`）。今天超標的生產函式逐一以「檔案路徑＋函式名」排除，清單只准刪不准加：新寫的函式超標，lint 紅；某一支拆小或刪掉之後排除還留著，golangci 印出 `Skipped 0 issues by rules`，CI 的 platform lint 步驟把那一行變成失敗。重跑名單：

```
golangci-lint run --enable-only=gocognit --max-same-issues 0 --max-issues-per-linter 0 ./... > gocognit.log
awk '/\(gocognit\)$/ && !/_test\.go/' gocognit.log
```

先寫進檔案再篩：直接接管線時，部分開發機的外掛會把 golangci 的輸出改寫成一行摘要（§0.3），篩出來是空的。`_test.go` 整批排除——超標的測試函式有二十幾支，排除規則因此一直有命中。

### 4.1 識別碼的三道守衛

| 守什麼 | 怎麼守 | 誰會紅 |
| --- | --- | --- |
| 鐵律 3：查詢預設帶工作區條件 | 吃參數、碰得到資料表、卻完全沒提 `workspace_id` 的查詢，必須在 `db/query-owners.yaml` 的 `scope:` 寫下理由 | `automation-check` 的 `query-scope` |
| 同種類識別碼不可換位 | 兩個同種類的值以角色名傳遞：`credit.LedgerQuery{Account, Workspace, Operator}`、`registry.VersionRange{From, To}`、`eval.RunPair{Run, Against}` | 三支整合測試（需要資料庫）：`TestCreditLedgerShowsTheGrantAndAuditsEveryRead`、`TestSkillDiffReportsAddedRemovedAndModifiedFilesInDirection`、`TestComparisonShowsBothVerdictsCostsAndTheVersionDiffLink` |
| 工作區識別碼排第一 | 函式參數裡的工作區 `pgtype.UUID` 排在其他 `pgtype.UUID` 之前；函式型別只要有兩個以上 `pgtype.UUID` 參數就必須寫出參數名字 | `automation-check` 的 `identifier-order` |

**`scope:` 的六種理由**——新增一支沒有工作區條件的查詢時，挑得出其中一種才可以宣告；挑不出來就是缺陷，停（§9）：

| 值 | 意思 |
| --- | --- |
| `user` | 按使用者本人算；識別碼來自 session，或由 session 推導 |
| `operator` | 只有 `RequireOperator` 路由或營運治理流程會呼叫，本來就跨租戶 |
| `worker` | 只有 Worker、maintenance、reconcile、purge 等系統行程會呼叫，沒有使用者請求路徑 |
| `content-addressed` | 以內容雜湊或 object key 定址，鍵本身不屬於任何工作區 |
| `scoped-upstream` | 識別碼只取自同一流程裡、已經帶工作區條件查出來的那一列（或寫入當下已授權的關聯，例如 fork 的來源版本）；**最弱的一種**，宣告前要指得出上游那一行 |
| `platform-wide` | 那張表沒有租戶欄位，參數是詞彙值不是實體識別碼 |

只呼叫 `pg_advisory_*`、不讀寫任何資料表的查詢由規則自動豁免，不必宣告。宣告過的查詢後來加上了工作區條件，檢查器會要求刪掉那筆宣告——宣告清單因此同時是檢查器的校準：偵測失明，每一筆都會變成過期；偵測過敏，每一支有條件的查詢都會變成未宣告。

---

## 5 已判定不做

每一條都附量測，因為判準是輸出不是偏好。要重開其中任何一條，先重跑它的 DISCOVER。這一節是裁決紀錄，不是待做清單。

### 5.1 artifact `kind` 型別化

`run_output`、`download_package` 在 Go 只以生成碼裡的字串存在，看起來像 J1 的目標。但**生產程式碼從來沒有讀過這個欄位**：每一支 query 都把 kind 寫死在 WHERE 裡，Go 沒有任何分支。

```
git grep -n "run_output\|download_package" -- apps/platform/internal/ | awk '!/_test/ && !/\/gen\//'
```

輸出為空。加型別會得到一個零呼叫者的抽象，J2 四問全否。若日後 Go 真的需要問「這個 artifact 是 run 產出嗎」，屆時依 §3 形狀補，並同時加進 `domain-vocabulary` 的對帳表。

### 5.2 Specification 的組合子與 `Rule[T]` 鏈

`create()` 的每個閘門在不同時點才拿得到自己要的事實：授權限制要先讀 skill，掃描結果要讀物件儲存，名額與額度要在交易裡。**沒有任何一段是兩條以上的規則吃同一組事實**，所以 `firstRefusal` 會是一個零呼叫者的泛型。

**規則改成入口唯一**：每一個拒絕都必須經過 `refused()`，那是同時遞增 `metrics.RunRefused` 與帶出稽核 reason 的唯一地方；`RefusalReasons()` 是完整詞彙。兩支測試守著——每個 `Reason*` 常數都要在名冊裡、任何 `refused()` 都不得把理由寫成字面值。

這條規則不是形式潔癖：繞過 `refused()` 的拒絕**進得了稽核卻進不了指標**，指標會安靜地少算。

### 5.3 每個 Service 一個回傳 `(*Service, error)` 的建構子

部分組裝是刻意的，不是疏漏。purge 路徑只給 `Pool` 與 `ClearSightings`，因為它的工作只讀這兩個欄位；要求 `packaging.Service` 十八個欄位全給的建構子，不是擋掉這些呼叫端，就是得為每種形狀再開一個建構子。

**部分組裝安全的判準是可達性**：worker 那個 `packaging.Service` 取用的每一個方法都追過，只讀 `Pool` 與 `ClearSightings`，兩個都已指派，沒有任何路徑讀到零值 `Retention`。要新增部分組裝就照這個方法追一次，不要靠「看起來欄位很多」下判斷。

守著接線的是反射測試——`apiserver/app_test.go` 與 `cmd/maintenance/main_test.go` 逐欄位斷言 `identity.Service` 沒有 nil 步驟。少接一條就紅。

**不讀任何欄位的方法寫成套件函式，不掛在 `*Service` 上**：空接收器（`func (*Service) …`）會逼呼叫端為了取用一個方法憑空生出空聚合根（`(&run.Service{}).X`）。這支指令應該沒有輸出：

```
git grep -nE "^func \(\*[A-Za-z]+\) " -- apps/platform/internal/ | awk '!/_test/ && !/\/gen\//'
```

### 5.4 把 `pgtype` 趕出領域簽名

**先講清楚這是什麼。** 是**識別碼在 Go 裡的型別**：今天工作區、Skill、版本、Run、使用者在 Go 全都是同一個 `pgtype.UUID`，也就是 pgx 驅動的型別。遷移是給每種實體一個自己的型別。

```go
func WorkspaceSkill(ctx context.Context, workspaceID, skillID pgtype.UUID) (Skill, bool, error)   // 今天
func WorkspaceSkill(ctx context.Context, workspaceID WorkspaceID, skillID SkillID) (Skill, bool, error)   // 遷移後
```

**不是 UUID 產生器，產生器不在範圍內。** 識別碼由 Postgres 自己產（27 處 `DEFAULT gen_random_uuid()`）；Go 只有三處 `uuid.NewString()`，產的是冪等鍵字串（`"grant:" + …`）不是實體識別碼。**也不動**資料庫欄位型別、欄位的值、migration 與 API 的 JSON——純粹是 Go 型別系統裡的事。

原本還有第二半：「把純粹為了轉手而存在的 `gen.DBTX` 參數拿掉」。**那一半是空集合。** 以 `gen.DBTX` 為參數的函式 29 個，13 個真的收過交易，16 個收的是連線——advisory lock（`LockObjectWrite`、`LockPackageObject`）與帳號清除迴圈必須在交易之外持有同一條連線。收連線不是轉手，是呼叫端在宣告自己控制著哪一條連線，那種看得見的邊界要留下。以下只談識別碼那一半。

不做的理由不是「太大」，是**它擋不住危害最大的那一類換位，而它擋得住的那一類已經有人擋了**。

型別化只能分辨**不同種類**的識別碼（workspace 對 skill）。同一種類的兩個值（兩個使用者、兩個版本、兩次 Run）在型別系統裡完全相同，換位照樣編譯。把每一種換位逐一追到後果：

| 換位種類 | 實測入口 | 後果 | 今天誰擋住 | 型別化擋得住？ |
| --- | --- | --- | --- | --- |
| 跨種類寫入 | 全部寫入路徑 | 交易回滾 | 外鍵：27 處 `REFERENCES workspaces (id)`；`creation_session_events`／`creation_receipts` 用複合外鍵 `(session_id, workspace_id)` | 是，但已有人擋 |
| 跨種類讀取（有 scope） | 絕大多數 | 查無資料 | `WHERE id = … AND workspace_id = …` | 是，但已有人擋 |
| 跨種類讀取（無 scope） | 111 支吃參數卻沒提 `workspace_id` 的查詢（98 支宣告理由、13 支只取 advisory lock 而豁免） | 識別碼若直接來自請求，會讀到別的工作區的列 | `query-scope`：每支都得寫下理由；逐支追過，沒有一支的識別碼直接來自請求（§4.1） | 是，但已有人擋 |
| 同種類 workspace↔workspace | `skill/library/registry.go` 的 `Fork`（`ws.ID` 對 `src.WorkspaceID`） | 查無資料 | `skill_id`＋`workspace_id` 聯合條件 | 否 |
| 同種類 user↔user | `creator/credit/service.go` 的 `Ledger`（被查的帳戶與查帳的操作員都是使用者） | 回傳錯的人的餘額，稽核列的 Actor 與 ResourceID 對調 | `LedgerQuery` 具名欄位＋整合測試（§4.1） | 否 |
| 同種類 version↔version | `skill/library/diff.go` 的 `DiffVersions`（from／to 都是版本） | diff 方向顛倒 | `VersionRange` 具名欄位＋整合測試（§4.1） | 否 |
| 同種類 run↔run | `trial/improvement/comparison.go` 的 `Comparison`（兩側都是 Run） | 比較兩側對調，`VersionDiffURL` 的 from／to 一起反向 | `RunPair` 具名欄位＋整合測試（§4.1） | 否 |

會產生「查得到、但答案是錯的」的有三處，型別化一處都擋不住，擋住它們的是具名欄位。型別化唯一獨到的價值落在無 scope 的那一列，而那一列已由 `query-scope` 逐支宣告理由擋住（§4.1）。

額度帳戶是 `credit_accounts.user_id PRIMARY KEY`，**按使用者算不是按工作區算**，所以 `Balance` 不比對工作區是設計正確。`Ledger` 要防的只是兩個使用者角色（被查的帳戶、查帳的操作員）互換。

**擋同種類換位的是具名欄位，不是具名型別。** 把並排的裸參數換成一個有欄位名字的結構，同種類與跨種類一起擋掉，成本是三支函式而不是 236 個轉換站點（§4.1）。

規模，供重開時參考：

```
git grep -oh "pgtype\.[A-Z][A-Za-z]*" -- apps/platform/internal/ | sort | uniq -c | sort -rn
git grep -n "gen\.[A-Za-z]*Params{" -- apps/platform/internal/ | awk '!/_test/ && !/\/gen\//' | wc -l
```

1630 處 `pgtype.UUID`、263 處 `pgtype.Timestamptz`，20 個套件。

**大在哪**：sqlc 生出來的參數結構永遠講 `pgtype.UUID`（那是它的工作，不該改），所以領域型別一換，**與生成碼交界的每一處都要轉一次**——遞進去要拆封，從生成的資料列讀出來要包回去。

```go
gen.GetSkillParams{ID: skillID.value(), WorkspaceID: workspaceID.value()}
```

這種 `gen.*Params{` 字面值站點，非測試非生成的有 **236 個**，反方向的讀取還不算。

便宜的別名接縫（`type ID = pgtype.UUID`）不成立——`Timestamptz` 會把 `pgtype` 的 import 留在原地，depguard 擋不掉。`pgtype.UUID` 也不擁有任何領域概念（C4／J1 問的是那個），它是 UUID 的容器。要做就是一次 ADR 加一次全 repo 遷移。

### 5.5 其餘九條中文領域 sentinel

`errors.New` 裡的中文共十三條，逐條追到寫入點再追到讀它的前端頁面。**四條是活文案**（前端真的印後端那句），已搬到寫它的 handler：`ErrUnsupportedType`、`ErrCreditBalance`、`ErrPreflightTargetNotFound` 的 GET `/runs/preflight` 路徑，以及 `errSkillNotFound` 走 `ReadFailure` 預設分支的那條。**其餘九條不搬**：

- `ErrNameTaken`、`errNoSavedVersion`、`errPackageUnreadable`、`ErrSuggestUnavailable` — 前端在那個狀態碼上寫死自己的句子，其中兩支前端測試明白斷言畫面**不**含後端原句。
- `ErrInvalid`、`ErrLimitExceeded` — handler 用 `stripSentinelPrefix` 把 sentinel 前綴剝掉，這兩句話本身永遠到不了 response body。
- `trial/design/http.go` 的 limit 訊息 — 前端恆送合法值，沒有呼叫端能觸發。
- `errSkillNotFound` — 前端確實會印它，但它所在的 `detail.go` 本身就是該 context 的 handler 層，搬了只是換個常數名。
- `ErrPreflightTargetNotFound` 的 POST `/runs` 路徑 — 前端寫死自己的句子；同一個 sentinel 的 GET 路徑是活的，已搬。

要重開任一條，先確認前端那一側改成印 `error.message` 了。

### 5.6 Go 只寫不讀的封閉詞彙

六個欄位的 `CHECK (… IN (…))` 是封閉詞彙，Go 會寫進去，但**沒有任何一處依它們的值分支**：`analytics_events.event_name`、`cost_events.ref_type`、`credit_entries.ref_type`、`creation_receipts.kind`、`evaluation_model_usage.operation`、`object_reconcile_sightings.resource_kind`。寫入端各是一組常數或兩處字面值，值寫進去之後沒有人讀回來做判斷，J1 問的那種領域問題不存在。

```
git grep -nE '(==|!=|case) *"(search_performed|skill_detail_viewed|session_started|download_started|creation_session|skill_version|operator_grant|command|attempt|judge|suggest|dataset|artifact)"' -- 'apps/platform/internal/*.go' | awk '!/_test\.go/ && !/\/gen\//'
git grep -nE '(==|!=|case) *(Event(SearchPerformed|SkillDetailViewed|SessionStarted|DownloadStarted)|Ref(CreationSession|Run|SkillVersion|OperatorGrant)|kind(Dataset|Artifact))\b' -- 'apps/platform/internal/*.go' | awk '!/_test\.go/ && !/\/gen\//'
```

兩支都應該沒有輸出。讀這些值的只有 SQL：`db/queries/cost.sql` 的會話加總以 `ref_type = 'creation_session'` 選列，與 §5.1 的 `kind` 同一形狀。

寫入端寫了 CHECK 不收的值時，交易會失敗；唯一例外是 `analytics_events`，寫入失敗只記一行 log（`analytics event not recorded`）不回報，所以新增事件卻忘了 migration 時只有 log 看得到。

`domain-vocabulary` 的 `unreconciledVocabularies` 逐筆列出這六個欄位，以及同一個形狀的 `skill_runtime_compatibility.runtime`：Go 只把它讀出來顯示量測結果，不依它分支，寫入端只有操作員手跑的 SQL。**重開條件**：Go 開始依其中任何一個值分支——屆時照 §3 形狀在擁有者型別化、接進對帳，並刪掉那一筆（不刪，檢查器會說它過期）。

### 5.7 creation 的 Session aggregate 與金額的 value type

creation 會話不包成 aggregate：唯一的寫入點 `advance()` 已經存在，改寫只換呼叫語法。金額不做 value type：運算已集中在 `money.go`，沒有混用過的事故，理由同 §5.4。**重開條件**：出現第二個寫入會話快照的地方，或金額在 `money.go` 之外被運算。

### 5.8 模型閘道的哨兵錯誤

`trial/execution/provider.go` 的四個領域錯誤有呼叫者：`retryable`、`refusedForCapacity`、`providerForgotAttempt` 各自依種類分支。模型閘道沒有這種呼叫者——四個呼叫點對錯誤的處理完全不看種類：

```
git grep -n "Gateway\." -- apps/platform/internal/trial/execution/ | awk '!/_test/ && !/gateway.go/'
```

排程把錯誤原樣往上拋、結算記一筆警告後跳過這個 attempt、改派一律轉成 `errSpendUnreadable`（它本來就是領域錯誤）。再加「閘道不回應」「閘道拒絕」這類哨兵，會得到零呼叫者的抽象，J2 四問全否。哪天真的要分辨（例如額度用盡不該重試、閘道暫時不通應該重試），照 `provider.go` 的 `Unwrap()` 形狀補，並同批補上會紅的測試。

---

## 6 待做

待做是 `04` 的丙-250～丙-253：四條對外邊界還沒到 `apps/platform/internal/trial/execution/provider.go` 的標準（領域動詞、可 `errors.Is` 的領域錯誤、HTTP status 與廠商型別只在 adapter）。判準是三問中兩問為是：**這個外部系統會被換掉嗎**、**領域程式是不是在講它的話**、**要測一條領域規則是不是得把外部系統架起來**。物件儲存（六個消費者各自宣告自己要的窄介面）、Outbox（`Insert` 與 `Dispatcher` 都是純 Go，只有排程觸發器碰佇列）、Trace（遮罩與 schema 驗證在領域層、寫入前無條件執行）與創作的抓取器（函式欄位）同批盤點過，判定已經合格。四項動工時各自照本節末尾的形狀寫進來。丙-249（模型閘道）已結案，它動工時放在這裡的規格隨結案移除，改用的判斷記在 §5.8。

[Aggregate 與領域事件](../adr/README.md#aggregate-與領域事件)的 Evaluation、Skill、Run aggregate 與 creation 的兩個具名概念都已改完；新的 aggregate 照抄 `trial/improvement/evaluation.go`（aggregate 與事件）、`evaluation_store.go`（載入與存回）、`evaluation_test.go`（只看唯讀狀態與事件）；aggregate 之間的事件往來照抄 `trial/improvement/mailbox.go`（訂閱者把事件投進 Mailbox，worker 消化，最後一次仍失敗才稽核）：

- 狀態不匯出，只有唯讀存取。命令不回傳值、不帶 `context`、不做 I/O：成立就改狀態並記下領域事件，不成立就只記一則帶理由的拒絕事件。
- 載入是吃呼叫端交易的套件函式並以列鎖讀出；存回只有一處，同交易寫狀態並把事件寫進 outbox；拒絕不存回。
- Aggregate 之間只用事件：outbox → Dispatcher 的訂閱 → 訂閱者的 Mailbox（River 佇列）→ 消化方法。
- 測試不連資料庫，只斷言唯讀狀態與事件。SQL 的原子性守衛（C2）全部保留。
- 新事件照[事件目錄](../../contracts/events/domain-events.md) §4 規則 4：目錄、outbox 常數、新 migration 的值域檢查、producer 同一個 commit。

新的待做先在 `04` 登記，再寫進這一節，每一項用同一個形狀：**GOAL**（要擋住什麼）、**DISCOVER**（能重跑的指令）、**EDIT**（改動的形狀）、**PROVE**（弄壞哪一行、哪條測試會紅）、**STOP-IF**（什麼情況停下回報）。

---

## 7 驗證指令

```
# 建置與靜態檢查（在 apps/platform）
go build ./...
gofmt -l <你改的檔>
go vet ./internal/<pkg>/
golangci-lint run ./internal/<pkg>/...

# 測試（不需要資料庫）
go test -count=1 ./internal/<pkg>/
go test ./internal/...

# 整合測試（需要資料庫）
task dev
export SKILLHUB_TEST_DATABASE_URL="postgresql://skillhub:skillhub@127.0.0.1:5432/skillhub_test"
export SKILLHUB_REQUIRE_DB=1
go test -count=1 -run '<Pattern>' ./internal/entrypoint/api/apiserver/

# 治理檢查
go -C tools/devctl run . comment-lint <你改的路徑>
go -C tools/devctl run . automation-check
task gen:check

# 改到 tools/devctl（例如 §4.1 的檢查器）時，在 tools/devctl
golangci-lint run
golangci-lint fmt --diff
go test -count=1 ./...
```

**已知陷阱**

- `SKILLHUB_TEST_DATABASE_URL` 的**資料庫名必須以 `_test` 結尾**，否則測試直接 panic。那是破壞性 migration 的守衛，不要指向開發資料庫。
- **不要自行釘住舊的 `GOTOOLCHAIN`**：`go.mod` 的下限高於它時會直接失敗。用預設工具鏈，版本來源見 `tools/toolchain.yaml`。
- 沒有設 `SKILLHUB_TEST_DATABASE_URL` 時整合測試會**跳過而不是失敗**，那是假綠。上面的 `SKILLHUB_REQUIRE_DB=1` 就是為此：缺資料庫的那一次跑會直接失敗，而不是全數跳過還回報 ok。宣稱整合通過前先確認那個套件回報的通過筆數不是零。**不要靠數 `-v` 的 `=== RUN` 行數**——那個輸出在部分開發機被外掛改寫（§0.3），會數到零而測試其實跑了；用 `-run` 指名單一測試看它自己的結果比較可靠。
- `task dev` 只起 Postgres 與 SeaweedFS，不花錢；`task dev:model` 會產生費用，唯讀子代理不得自行啟動。
- 本機可能有其他專案的容器在跑，名稱不以 `skillhub-` 開頭的一律不得使用。

---

## 8 突變協定

每一條你宣稱修好的東西，逐條做：

1. 把修正的**那一行**還原成錯的樣子。
2. 只跑對準它的那一條測試。
3. 確認 **FAIL**，記下訊息。
4. 改回來。
5. `git diff` 必須為空。

**不適用**：純文案、純註解。
**例外**：修的是測試之間的依賴時，單改一行不會紅，改用受控重現（刻意製造失敗條件，修正前紅、修正後綠，最後移除鷹架）。

**守衛類改動要做兩種突變**，缺一不可：

- **表被改壞** → 真實流程的整合測試必須紅，而且錯誤訊息必須是守衛自己丟的那一句。這證明**守衛真的接在寫入路徑上**。
- **規則被改錯** → 對應的純測試必須紅。這證明**規則本身有人守**。

**回報一行一條**（`T 編號`＝ [`istqb-test-design`](../../.claude/skills/istqb-test-design/SKILL.md) 九條判準的編號，例如 T3 邊界值、T5 狀態轉移）

`<測試名> — <T 編號>：<條件>（產品 <檔案:行>）— 弄壞 <那一行> → FAIL（<訊息>）→ 還原 → diff 空`

---

## 9 停止與升級條件

遇到以下情形**停下並回報**，不要自行決定。

| 情形 | 為什麼要停 |
| --- | --- |
| 需要新增或修改 `db/migrations/`、`db/queries/`、`contracts/` | C5 高衝突區，由主 Agent 序列化 |
| 某條現行轉移看起來像 bug 而不是規則 | 把 bug 寫成規格與把規則寫成規格是兩件事，需要人裁定 |
| 收緊某個守衛會讓現行流程被擋 | 那是行為改變，不是重構 |
| 合併閘門會改變對外的 reason 碼 | 對外契約 |
| 需要改[Platform Bounded Context 與 Context Map](../adr/README.md#platform-bounded-context-與-context-map)或[Query 與寫入所有權](../adr/README.md#query-與寫入所有權)的既有決策 | 結構性偏離先更新 ADR，且 ADR 不原地改寫，要開新的 |
| DISCOVER 的輸出與本檔描述不符 | 本檔過期，先對齊事實再動手 |
| 新增的查詢沒有工作區條件，也挑不出 §4.1 六種理由之一 | 那是鐵律 3 的缺陷，不是宣告問題；進 [`05`](../plans/05-pending-rulings.md) |

待裁定事項一律進 [`05`](../plans/05-pending-rulings.md)，不要在程式碼裡自行決定。creation 的終態只認 `saved` 與 `cancelled`，不含 `failed`，因此 `failed` 的會話能接受的指令遠多於直覺——`raise_budget` 把失敗會話帶回 `waiting_input` 是[帳號清除與 Credit](../adr/README.md#帳號清除與-credit)的明文設計，其餘是這個定義的連帶結果。轉移表照現況記錄，**不得自行收緊**。

---

## 10 回報格式

每個任務結束回報四段，不要長篇。

1. **做了什麼** — 檔案清單 ＋ 一句話說明改動的形狀。
2. **驗證** — §7 各指令的結果；整合測試寫出那個套件回報的通過筆數，有跳過就說明為什麼（§7 已知陷阱第三條）。
3. **突變證明** — §8 格式，一行一條。
4. **停下來的地方** — §9 命中哪一條，需要誰裁定什麼。
