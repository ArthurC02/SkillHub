# DDD／Clean Code 收斂：Coding Agent 執行規格

本檔給要動 `apps/platform/internal/` 領域程式碼的 Coding Agent。日常的跨 context 判斷看 [platform-ddd-practices.md](platform-ddd-practices.md)；本檔是把領域規則從資料庫收回程式碼的執行規格。

動手前先讀根目錄 [`AGENTS.md`](../../AGENTS.md) 與 [`apps/platform/internal/AGENTS.md`](../../apps/platform/internal/AGENTS.md)，以及目標套件的 `doc.go`。

---

## 0 執行協定

1. **一次認領一個任務 ID**，做完 VERIFY 才能認領下一個。
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
| C8 | 每個宣稱修好的東西都要有一次紅（§7；[automation.md](automation.md) 開發自動化第 9 條）。 |
| C9 | 禁止 `git stash`。對不屬於自己的修改禁止 `git reset`／`git clean`／`git checkout -- <path>`。還原自己的檔案用 `git show HEAD:<path> > <path>`。 |
| C10 | 只以明確 pathspec stage，永不 `git add .`。 |
| C11 | 子代理預設唯讀，派工必須指定 model，禁用旗艦級；子代理不做 git 寫入。 |

---

## 2 判準

動手前先過判準。答「否」就不要做，並在回報裡寫出是哪一條擋下的。

**J1 領域概念歸屬**

> 這個領域問題（「這是終態嗎」「這個授權關了嗎」「這個 artifact 是 run 產出嗎」）能不能**在不連資料庫的情況下**回答？

不能 → SQL 擁有它 → 要補 Go 的定義。能 → 已經合格，不要動。

**J2 值物件四問**（要不要把一個裸型別包起來）

有規則？會跟同型別的別的值搞混？有運算？會跨邊界？**四問全否 → 不要包。**

**J3 `require*` 分類**（可機器判別）

- 無參數（`func (s *Service) requireX() error`）＝ **接線檢查**，屬於 T5。
- 吃事實（`func (s *Service) requireX(facts …) error`）＝ **規格**，屬於 T3。

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
- 反假綠：斷言集合大小（`len(AllStates()) != 10` 就 Fatal）。

**特徵化先於強制**

沒有書面規格時，轉移表只能從程式碼讀出來。**直接把讀到的東西寫成表，等於把現況裡的 bug 升格成規格。** 順序必須是：先追出每條路徑並寫測試釘住現況 → 再寫表 → 最後才開啟強制。追路徑派唯讀子代理平行做，一個檔一個代理，要求逐行閱讀並附 `檔案:行`。

---

## 4 任務圖

```
參考實作：creation 狀態機
 ├─→ T1 詞彙對帳檢查器        ← 先做，是其餘任務的機器保證
 ├─→ T2 SQL 獨佔概念收回 Go
 ├─→ T4 evaluation 狀態機
 └─→ T3 Specification 統一
T5 建構安全（獨立）
T6 outbox payload（獨立）
T7 身分型別（大，排在 T1 之後，逐 context）
T8 領域錯誤的使用者文案（獨立，先量測）
```

順序規則：**T1 先做。** 沒有對帳機器，其餘任務補的 Go 定義會再次與 SQL 分岔。

---

## 5 任務規格

### T1 詞彙對帳檢查器

**GOAL** 同一組封閉詞彙在 Go 常數、DB `CHECK (… IN (…))`、OpenAPI `enum` 三處不一致時，CI 紅。

**DISCOVER**

```
awk '/\{"/{print NR": "$0}' tools/devctl/automation_check.go
git grep -h "CHECK" -- db/migrations/ | awk '/IN \(/' | wc -l
git grep -n "enum:" -- contracts/openapi/public.yaml | wc -l
```

**範本**（照抄結構，不要自創）

- `tools/devctl/isolation_levels.go` — AST 抓常數（`parser.ParseFile` → `*ast.GenDecl`／`token.CONST` → `*ast.ValueSpec` → `*ast.BasicLit` → `strconv.Unquote`），regexp 讀 YAML `enum:`。
- `tools/devctl/route_table.go` — 同一種寫法，另一個對帳對象。
- `tools/devctl/query_owners.go` 的 `frozenTables()` — 掃全部 `db/migrations/*.sql` 的既有寫法。

**EDIT**

1. 新檔 `tools/devctl/domain_vocabulary.go`，簽名 `func domainVocabularyProblems(root string) []string`。
2. 詞彙以宣告表驅動，每組一列：Go 檔路徑、常數前綴、DB 欄位名、契約 schema key。第一列用 creation 的 `State`。
3. 註冊到 `documentCheckers()`：`{"domain-vocabulary", domainVocabularyProblems}`。
4. **不要引入 YAML 套件**，沿用 regexp。這個 repo 至今零 `packages.Load`，不要引入。

**VERIFY**

```
go -C tools/devctl run . automation-check
cd tools/devctl && go test ./...
```

**PROVE** 在 `state.go` 刪掉一個常數 → 檢查器必須指名該值缺席 → 還原 → `git diff` 空。
另加自我校驗測試（抄 `isolation_levels_test.go` 的 `TestTheRealRepositoryHasNoIsolationDrift`）：斷言常數數量下限與契約值存在，**防止檢查器空轉卻回報通過**。

**STOP-IF** 某組詞彙 DB 側沒有 CHECK（`creation_sessions.state` 就沒有）。**不要為了讓檢查器通過而新增 migration**，那是 C5 的高衝突區。檢查器要能容忍「DB 側不存在」並把它列為待補，不是列為錯誤。

---

### T2 SQL 獨佔的領域概念收回 Go

**GOAL** 讓 J1 對這三組概念答「能」。**SQL 側一條都不刪**（C1）。

**DISCOVER**

```
git grep -n "object_grants_state" -- db/ apps/platform
git grep -c "'succeeded', 'failed', 'cancelled', 'timed_out'" -- db/
git grep -n "'run_output'\|'download_package'" -- db/queries db/migrations
```

#### T2a `object_grants_state`

四個值 `legacy_unknown`／`unissued`／`recorded`／`closed`，定義在 `db/migrations/0050_run_attempt_object_grant_expiry.sql`。Go 端零具名、零判斷、零單元測試，唯一引用是 sqlc 生成的 `ObjectGrantsState string`。

真實轉移由四支 query 決定：

| query | 轉移 |
| --- | --- |
| `CreateRunAttempt` | `(新列) → unissued` |
| `FinishRunAttempt` | `unissued → closed`，其餘原樣 |
| `CloseUnissuedRunAttemptGrants` | `unissued → closed` |
| `SetRunAttemptObjectGrantsExpiry` | `(任意) → recorded`，**SQL 無 state 前置條件** |

`legacy_unknown` 在現行呼叫圖中沒有離開路徑（`TestLegacyAttemptGrantStateRemainsFailClosed` 斷言它 fail closed）。`recorded` 無法回到 `unissued`。

**EDIT** 在 `apps/platform/internal/trial/execution/` 新增 `grantstate.go`，照 §3 形狀。呼叫端在呼叫 `SetRunAttemptObjectGrantsExpiry` 前先問轉移表——那支 query 沒有 state 前置條件，Go 這一側就是它唯一的守門。

**注意** `object_grants_expire_at` 與狀態是一組：`unissued` 配 `infinity`；轉 `closed` 時回填 `now()-2min`；`recorded` 的有效性完全由 `expire_at` 決定。過 J2 後把兩者包成一個值物件。

#### T2b `IsTerminal` 的 SQL 展開

Go 定義已存在（`apps/platform/internal/trial/execution/statemachine.go`）。SQL 裡另有十餘處字面列舉，分佈在 migration 與 query 兩側，實際數量以 DISCOVER 輸出為準。

**不要改 SQL。** 改法是讓 T1 的檢查器把「Go 的終態集合」與「SQL 每一處 `IN ('succeeded', …)` 的值集」對帳，不一致就紅。新增終態時一次改齊。

#### T2c artifact `kind`（已判定不做）

`run_output`、`download_package` 在 Go 只以生成碼裡的字串存在，看起來像 J1 的目標。但**生產程式碼從來沒有讀過這個欄位**：每一支 query 都把 kind 寫死在 WHERE 裡，Go 沒有任何分支。DISCOVER 確認：

```
git grep -n "run_output\|download_package" -- apps/platform/internal/ | awk '!/_test|\/gen\//'
```

輸出為空。加型別會得到一個零呼叫者的抽象，J2 四問全否。**這一項關閉。** 若日後 Go 真的需要問「這個 artifact 是 run 產出嗎」，屆時再依 §3 形狀補，並同時加進 T1 的對帳表。

**VERIFY** 該 context 測試綠；`go test ./internal/...` 零失敗；整合測試（§6）綠。
**PROVE** 每個新轉移表各弄壞一列，對應測試必須紅。

---

### T3 Specification 統一

**GOAL** 把散在四種慣例裡的守門判斷收成一種可組合、可單元測試的形狀。

**DISCOVER**

```
git grep -n "^func (s \*Service) require" -- apps/platform/internal/ | sed 's/(ctx.*//'
git grep -n "(reason, message string)" -- apps/platform/internal/
git grep -n "type refusal struct" -- apps/platform/internal/
```

**分類（用 J3）**

- **吃事實的＝規格**：`requireNotAccessRestricted`、`requireScanNotBlocking`、`requireRunSlot`、`requireDispatchable`、`requireQuota`、`requireCredit`、`requirePermissionConfirmation`、`requireCuratedContent`、`requireGenerateAllowance`。
- **無參數的＝接線檢查**，不屬於本任務，移交 T5：`requireProjection`、`requireTestLab`、`requireRunLinks`、`requirePurgeSteps`、`requirePurgeReads`、`requireOwnerReads`。

**既有資產，不要重造** `apps/platform/internal/trial/execution/gateb.go` 的 `refusal{reason, err}`；`refused()` 會遞增 `metrics.RunRefused.WithLabelValues(reason)`；reason 在 `service.go` 被取出寫進 `audit.ActionRunRefused`。這已經是 Specification 的八成。

**目標形狀**

```
type Refusal struct { Reason string; Err error }
type Rule[T any] func(T) *Refusal
func firstRefusal[T any](subject T, rules ...Rule[T]) *Refusal
```

- **純規格**（事實進、拒絕出）與 **I/O 閘門**（要查 DB 或外部）分開放。純規格必須能不連資料庫測試。
- 鏈的順序寫成一份**可斷言的清單**，不要藏在 `create()` 的呼叫順序裡。

**這是行為改變，不是重構。** 閘門順序決定哪個拒絕理由先浮出來，而 reason 碼直接餵指標與稽核紀錄。
**必做前置**：先寫一支測試把現行順序釘死（每一對閘門同時觸發時誰先贏），這支先綠，才可以動結構。

**不做** And／Or／Not 組合子樹。線性鏈涵蓋全部現存用法，組合子是為了還不存在的需求。

**推進順序** `trial/execution` → `skill/delivery`（把 `gate`／`gateFlags` 的 `(reason, message string)` 併過來）→ `creator/creation` → `skill/admission`、`skill/library`。一個 context 一批。

**STOP-IF** 合併兩個閘門會改變對外的 reason 碼字面值 → 停，回報，那是對外契約。

---

### T4 evaluation 狀態機

**GOAL** 照 §3 形狀補 `pending`／`completed`／`failed`。

**DISCOVER**

```
git grep -n "StatusPending\|StatusCompleted\|StatusFailed" -- apps/platform/internal/trial/improvement/
git grep -n "CHECK (status IN" -- db/migrations/0024_evaluation.sql
```

DB 有 `CHECK (status IN ('pending','completed','failed'))`，Go 有三個**未定型**常數，狀態判斷散在數處 `current.Status == StatusPending`。

**EDIT** 型別化三個常數、加轉移表、把散落的比較收成一個具名謂詞、註冊進 T1 的對帳表。規模遠小於 creation，照抄即可。

---

### T5 建構安全

**GOAL** 用編譯期保證換掉執行期 nil 檢查。

**DISCOVER**

```
git grep -c "^type Service struct" -- apps/platform/internal/ | wc -l
git grep -n "^func NewService\|^func New(" -- apps/platform/internal/ | awk '!/_test/'
git grep -n "not configured" -- apps/platform/internal/ | awk '!/_test/' | wc -l
git grep -n "packaging.Service{" -- apps/platform/
```

`Service` 型別遠多於建構子。既有的 `service-construction` 檢查器管的是 [ADR-032](../adr/ADR-032-ddd-bounded-context-governance-for-platform.md) 的跨 context 現場建構，**不管自家建構子**，所以這一項沒有機器守著。

**先修真實地雷**：worker 裡有一個半組裝的 `packaging.Service`，必填欄位多數從未指派。這不是理論風險。

**EDIT** 一個 context 一個 commit。建構子收必填欄位、回傳 `(*Service, error)`，刪掉該 context 因此變成不可能的 nil 檢查與「未配置」sentinel。注入點只在 `entrypoint/api/apiserver.NewApp`。

**PROVE** 拿掉建構子裡某個必填欄位的檢查 → 對應測試紅。

---

### T6 outbox payload

**GOAL** `NewEvent.Payload []byte` 換成具名 struct，並加 `schema_version`（[ADR-008](../adr/ADR-008-asynchronous-workflows-and-domain-events.md)）。

**DISCOVER**

```
git grep -n "outbox.Insert\|NewEvent{" -- apps/platform/internal/ | awk '!/_test/'
```

現在加是一行；有第三個消費者之後再加是一次遷移。

---

### T7 身分型別

**GOAL** 把 `pgtype.UUID`／`pgx.Tx` 趕出**領域**簽名。

**DISCOVER**

```
git grep -c "pgtype.UUID" -- apps/platform/internal/ | awk -F: '!/_test|\/gen\//{s+=$2} END {print s}'
git grep -n "pgx.Tx\|gen.DBTX" -- apps/platform/internal/ | awk '!/_test|\/gen\//' | wc -l
```

兩個 `awk` 都必須排除 `/gen/`。生成的持久層本來就該講 `pgtype`，把它算進來會讓數字膨脹一倍並指向錯的檔案。

**邊界，不要做過頭**

- **要趕走**：領域型別欄位與回傳值裡的 `pgtype.UUID`；純粹為了轉手而出現的 `gen.DBTX`。
- **要留下**：`Act(ctx, tx, …)` 這種**明示交易邊界**的參數。呼叫端必須看得見自己在一個交易裡，那是守鐵律 9 的手段。

逐 context 進行，一個 context 一個 commit。排在 T1 之後。

---

### T8 領域錯誤的使用者文案

**GOAL** 先量測，再決定搬不搬。**不要一次搬。**

**DISCOVER**

```
git grep -n "errors.New(\"" -- apps/platform/internal/ | awk '!/_test/ && /[一-龥]/'
```

這支指令指的是**領域 sentinel 裡的中文**，那是本任務的目標。不要用更寬的「掃所有中文字串」去量——那會撈到 API 層的 `httpx.WriteError` 文案，那些本來就該是中文，不是缺陷。

中文使用者文案寫在領域 sentinel 裡，會原樣進 HTTP body；其中有些是死文字，因為前端在對應狀態碼上用自己的文案。

**EDIT 前先做**：對每一條查前端是否真的顯示它——拿那句中文去 `git grep -- apps/web/src`，找不到就是死文字。只搬前端真的會顯示的那些。

---

## 6 驗證指令

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
go test -count=1 -run '<Pattern>' ./internal/entrypoint/api/apiserver/

# 治理檢查
go -C tools/devctl run . comment-lint <你改的路徑>
go -C tools/devctl run . automation-check
task gen:check
```

**已知陷阱**

- `SKILLHUB_TEST_DATABASE_URL` 的**資料庫名必須以 `_test` 結尾**，否則測試直接 panic。那是破壞性 migration 的守衛，不要指向開發資料庫。
- **不要自行釘住舊的 `GOTOOLCHAIN`**：`go.mod` 的下限高於它時會直接失敗。用預設工具鏈，版本來源見 `tools/toolchain.yaml`。
- 沒有設 `SKILLHUB_TEST_DATABASE_URL` 時整合測試會**跳過而不是失敗**，那是假綠。宣稱整合通過前先用 `-v` 數 `=== RUN` 的筆數。
- `task dev` 只起 Postgres 與 SeaweedFS，不花錢；`task dev:model` 會產生費用，唯讀子代理不得自行啟動。
- 本機可能有其他專案的容器在跑，名稱不以 `skillhub-` 開頭的一律不得使用。

---

## 7 突變協定

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

**回報一行一條**

`<測試名> — <T 編號>：<條件>（產品 <檔案:行>）— 弄壞 <那一行> → FAIL（<訊息>）→ 還原 → diff 空`

---

## 8 停止與升級條件

遇到以下情形**停下並回報**，不要自行決定。

| 情形 | 為什麼要停 |
| --- | --- |
| 需要新增或修改 `db/migrations/`、`db/queries/`、`contracts/` | C5 高衝突區，由主 Agent 序列化 |
| 某條現行轉移看起來像 bug 而不是規則 | 把 bug 寫成規格與把規則寫成規格是兩件事，需要人裁定 |
| 收緊某個守衛會讓現行流程被擋 | 那是行為改變，不是重構 |
| 合併閘門會改變對外的 reason 碼 | 對外契約 |
| 需要改 [ADR-032](../adr/ADR-032-ddd-bounded-context-governance-for-platform.md)、[ADR-033](../adr/ADR-033-sqlc-query-ownership-and-cross-context-write-enforcement.md)、[ADR-035](../adr/ADR-035-read-ownership-enforcement-and-context-map-completeness.md) 的既有決策 | 結構性偏離先更新 ADR，且 ADR 不原地改寫，要開新的 |
| DISCOVER 的輸出與本檔描述不符 | 本檔過期，先對齊事實再動手 |

待裁定事項一律進 [`05`](../plans/05-pending-rulings.md)，不要在程式碼裡自行決定。creation 的終態只認 `saved` 與 `cancelled`，不含 `failed`，因此 `failed` 的會話能接受的指令遠多於直覺——`raise_budget` 把失敗會話帶回 `waiting_input` 是 [ADR-068](../adr/ADR-068-credit-is-the-only-unit-of-account.md) 的明文設計，其餘是這個定義的連帶結果。轉移表照現況記錄，**不得自行收緊**。

---

## 9 回報格式

每個任務結束回報四段，不要長篇。

1. **做了什麼** — 檔案清單 ＋ 一句話說明改動的形狀。
2. **驗證** — §6 各指令的結果；整合測試要寫出實際跑了幾支、跳過幾支。
3. **突變證明** — §7 格式，一行一條。
4. **停下來的地方** — §8 命中哪一條，需要誰裁定什麼。
