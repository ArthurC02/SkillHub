# DDD／Clean Code 收斂：Coding Agent 執行規格

本檔給要動 `apps/platform/internal/` 領域程式碼的 Coding Agent。日常的跨 context 判斷看 [platform-ddd-practices.md](platform-ddd-practices.md)；本檔是把領域規則從資料庫收回程式碼的執行規格。

動手前先讀根目錄 [`AGENTS.md`](../../AGENTS.md) 與 [`apps/platform/internal/AGENTS.md`](../../apps/platform/internal/AGENTS.md)，以及目標套件的 `doc.go`。

---

## 0 執行協定

1. **一次做一件事**，§6 的驗證全綠才開始下一件。
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
- 反假綠：斷言集合大小（`len(AllStates()) != 10` 就 Fatal）。

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
| run 被拒絕的理由詞彙 | `trial/execution/specification.go` 的 `RefusalReasons()` | `specification_test.go` | 同套件的 AST 測試 |
| 額度拒絕理由詞彙 | `product/entitlements` 的 `AllowanceRefusalReasons` | `quota_test.go` | 同上 |
| outbox 事件 payload 的形狀 | `foundation/messaging/outbox/events.go` | `payload_test.go` | — |
| 使用者看得到的句子 | 寫出它的 handler，不是領域 sentinel | `messages_test.go`（`trial/design`、`trial/execution`） | 同上 |

`devctl automation-check` 有兩個檢查器守這張表：

- `domain-vocabulary` — Go 常數 ↔ DB `CHECK (… IN (…))` ↔ Postgres enum 三處對帳，缺一側時列為待補而不是錯誤。
- `run-status-sql` — Go 的 `successors` ↔ migration 0032 的 trigger 轉移列 ↔ 每一處終態 `IN` 清單。

閘門的順序仍然寫在 `create()` 的呼叫序，那是刻意的：順序決定哪個 reason 先浮出來，而 reason 直接餵 `metrics.RunRefused` 與 `audit.ActionRunRefused`。要改順序就是改對外行為，先看 §8。

C1 成立：SQL 側沒有任何 constraint、trigger、unique 或外鍵被刪除或放寬。

---

## 5 已判定不做

每一條都附量測，因為判準是輸出不是偏好。要重開其中任何一條，先重跑它的 DISCOVER。

### 5.1 artifact `kind` 型別化

`run_output`、`download_package` 在 Go 只以生成碼裡的字串存在，看起來像 J1 的目標。但**生產程式碼從來沒有讀過這個欄位**：每一支 query 都把 kind 寫死在 WHERE 裡，Go 沒有任何分支。

```
git grep -n "run_output\|download_package" -- apps/platform/internal/ | awk '!/_test|\/gen\//'
```

輸出為空。加型別會得到一個零呼叫者的抽象，J2 四問全否。若日後 Go 真的需要問「這個 artifact 是 run 產出嗎」，屆時依 §3 形狀補，並同時加進 `domain-vocabulary` 的對帳表。

### 5.2 Specification 的組合子與 `Rule[T]` 鏈

`create()` 的每個閘門在不同時點才拿得到自己要的事實：授權限制要先讀 skill，掃描結果要讀物件儲存，名額與額度要在交易裡。**沒有任何一段是兩條以上的規則吃同一組事實**，所以 `firstRefusal` 會是一個零呼叫者的泛型。

真正的缺陷不是形狀而是入口：曾經有兩個理由（`permissions_unconfirmed`、`capability_mismatch`）靠 `auditRefusal` 裡的 `errors.Is` 階梯補回名字，因此進得了稽核卻進不了指標。現在所有拒絕都經 `refused()`，`RefusalReasons()` 是完整詞彙，兩支測試守著：每個 `Reason*` 常數都要在名冊裡、任何 `refused()` 都不得把理由寫成字面值。

### 5.3 每個 Service 一個回傳 `(*Service, error)` 的建構子

部分組裝是刻意的，不是疏漏。purge 路徑只給 `Pool` 與 `ClearSightings`，因為它的工作只讀這兩個欄位；要求 `packaging.Service` 十八個欄位全給的建構子，不是擋掉這些呼叫端，就是得為每種形狀再開一個建構子。

worker 那個 `packaging.Service` 不是地雷：從它取得的每一個方法都追過，只讀 `Pool` 與 `ClearSightings`，兩個都已指派，沒有零值 `Retention` 的路徑。

守著接線的是反射測試——`apiserver/app_test.go` 與 `cmd/maintenance/main_test.go` 逐欄位斷言 `identity.Service` 沒有 nil 步驟。少接一條就紅。

同一批把十二個空接收器方法（`func (*Service) …`，完全不讀欄位）改成套件函式，`(&run.Service{}).SkillVersionsInRuns` 這種為了取用方法而憑空生出的空聚合根因此消失。

### 5.4 把 `pgtype` 趕出領域簽名

原本設想「逐 context 一個 commit」。量測之後兩半都不成立。

**`gen.DBTX` 那一半是空的。** 以它為參數的函式有 29 個，逐一追呼叫端：13 個真的收過交易，16 個收的是連線。收連線的那些也不是轉手——advisory lock（`LockObjectWrite`、`LockPackageObject`）與帳號清除迴圈必須在交易之外持有同一條連線，那是呼叫端在宣告自己控制著哪一條連線，看得見的邊界要留下。**沒有一個是純粹為了轉手而存在。**

**`pgtype.UUID` 那一半太大且接縫不成立。**

```
git grep -oh "pgtype\.[A-Z][A-Za-z]*" -- apps/platform/internal/ | sort | uniq -c | sort -rn
```

1630 處 `pgtype.UUID`、263 處 `pgtype.Timestamptz`，橫跨 20 個套件；非測試非生成的部分就有 160 個結構欄位與 178 個回傳簽名。唯一便宜的做法是在自家套件宣告 `type ID = pgtype.UUID` 這種別名接縫，但 `Timestamptz` 會跟著留下，套件仍得 import `pgtype`，depguard 擋不掉，接縫的好處歸零。改成真正的領域 ID 值物件則要在每個生成列的邊界寫轉換——**那次遷移的成本，等於它要預防的那次更換驅動的成本。**

`pgtype.UUID` 也不擁有任何領域概念（C4／J1 問的是那個），它是 UUID 的容器。要做就是一次 ADR 加一次全 repo 遷移，不是這份規格的任務。

### 5.5 其餘九條中文領域 sentinel

`errors.New` 裡的中文共十三條，逐條追到寫入點再追到讀它的前端頁面。四條是活文案（前端真的印後端那句），已搬到寫它的 handler。其餘九條不搬：

- `ErrNameTaken`、`errNoSavedVersion`、`errPackageUnreadable`、`ErrSuggestUnavailable` — 前端在那個狀態碼上寫死自己的句子，其中兩支前端測試明白斷言畫面**不**含後端原句。
- `ErrInvalid`、`ErrLimitExceeded` — handler 用 `stripSentinelPrefix` 把 sentinel 前綴剝掉，這兩句話本身永遠到不了 response body。
- `trial/design/http.go` 的 limit 訊息 — 前端恆送合法值，沒有呼叫端能觸發。
- `errSkillNotFound` — 它所在的 `detail.go` 本身就是該 context 的 handler 層，搬了只是換個常數名。
- `ErrPreflightTargetNotFound` 的 POST `/runs` 路徑 — 前端寫死自己的句子；同一個 sentinel 的 GET 路徑是活的，已搬。

要重開任一條，先確認前端那一側改成印 `error.message` 了。

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
