# apps/platform/internal — 動手前的指標

**只放指標，不複製對照表、不抄數字**：不一致時以被指的那一份為準（被指的那幾份有機器對帳，這一份沒有）。

- **這支檔案該放哪、屬於哪個邊界**：先看 [README.md](./README.md)（產品價值流導覽），邊界類型與逐 package 對照以 [Domain Memory](../../../docs/domain-memory/) 的已審查 Context（Core 與 Supporting）與 [`architecture-identity.yaml`](../architecture-identity.yaml)（Shared Kernel 與 Generic）為準，兩者都受 `devctl automation-check` 的 `context-map` 對帳。**新套件先登記，再建目錄**——反過來做會在你寫第一行程式之前就紅。
- **每個套件的 `doc.go` 一句話說這個 context 擁有什麼**（最多 3 行，受 `comment-budget` 管）。擁有哪些表看 `db/query-owners.yaml`，允許的協作方向看 [Domain Memory](../../../docs/domain-memory/) 的已審查 dependency policy 與 `apps/platform/.golangci.yml` 的 depguard——兩者由 `automation-check` 的 `depguard-deny` 對帳，doc.go 不重述。
- **要拿別的 context 的事實**：看 [platform-ddd-practices.md](../../../docs/development/platform-ddd-practices.md)〈跨 Context 協作〉，兩種寫法的判準在〈同步 owner 讀取的兩種形狀〉；決策見 [Query 與寫入所有權](../../../docs/adr/README.md#query-與寫入所有權) 與 [Aggregate 與領域事件](../../../docs/adr/README.md#aggregate-與領域事件)（aggregate 之間用領域事件，其餘跨 context 寫入用反轉；讀取是同一手法的鏡像）。**generated row 不跨界**。
- **不要在方法內建構別人的 `Service`**，也不要靠讀對方的內部欄位確認它接好了。前者由 `automation-check` 的 `service-construction` 擋，後者沒有機器守著。
- **新增／刪除 `db/queries/*.sql` 的 query**：同一批改 `db/query-owners.yaml`。`allow:`／`read_allow:` 是遷移完成後的空清單，**不是擴充點**——新協作走 owner 的 Service API。
- **跨 context 的新 import**：同一個 commit 在已審查的 Registry 立下 dependency policy，並改 `apps/platform/.golangci.yml` 的 depguard 規則（`depguard-deny` 要求兩邊一致，少哪一邊都會紅）。
- **不屬於單一寫入者的東西**：`contracts/`、`db/migrations/`、`db/queries/`、`db/query-owners.yaml` 與所有 generated 目錄由主 Agent 序列化。你的改動若需要動到其中任何一個（新 endpoint、新 query、schema 變更），停下來回報，不要自己改也不要繞過。
- **改完跑**：`go -C tools/devctl run . automation-check` ＋ 受影響套件的測試。**`go test ./...` 在沒有 `SKILLHUB_TEST_DATABASE_URL` 時是假綠**——integration 測試會 skip，那個 ok 是幾百條斷言拒絕執行。

**這個檔是給所有 coding agent 讀的**（Claude Code 讀不到 `AGENTS.md`，所以同目錄的 `CLAUDE.md` 只有一行 `@AGENTS.md` 把它 import 進來）。內容只寫在這一份，不複製。
