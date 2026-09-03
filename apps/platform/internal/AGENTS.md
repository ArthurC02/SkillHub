# apps/platform/internal — 動手前的指標

**只放指標，不複製對照表、不抄數字**：不一致時以 ADR 為準（ADR 那一份有機器對帳，這一份沒有）。

- **這支檔案該放哪、屬於哪個邊界**：先看 [README.md](./README.md)（產品價值流導覽），邊界類型與逐 package 對照表以 [ADR-032](../../../docs/adr/ADR-032-ddd-bounded-context-governance-for-platform.md) §1 為準（受 `devctl automation-check` 的 `context-map` 對帳）。**新套件先在 §1 登記，再建目錄**——反過來做會在你寫第一行程式之前就紅。
- **每個套件的 `doc.go` 是離你最近的事實來源**：它寫了這個 context 擁有哪些事實、允許哪些協作方向與理由、以及**刻意不放在這裡**的東西。動它之前先讀那一段，比讀 ADR 快也比較準。
- **要拿別的 context 的事實**：看 [platform-ddd-practices.md](../../../docs/development/platform-ddd-practices.md)〈跨 Context 協作〉，兩種寫法的判準在〈同步 owner 讀取的兩種形狀〉；決策是 [ADR-034](../../../docs/adr/ADR-034-cross-context-writes-close-by-inversion-not-by-events.md)（寫入用反轉，不用事件；讀取是同一手法的鏡像）。**generated row 不跨界**。
- **不要在方法內建構別人的 `Service`**，也不要靠讀對方的內部欄位確認它接好了。前者由 `automation-check` 的 `service-construction` 擋，後者沒有機器守著。
- **新增／刪除 `db/queries/*.sql` 的 query**：同一批改 `db/query-owners.yaml`。`allow:`／`read_allow:` 是遷移完成後的空清單，**不是擴充點**——新協作走 owner 的 Service API。
- **跨 context 的新 import**：同一個 commit 改 ADR-032 附錄 A 與 `apps/platform/.golangci.yml` 的 depguard 規則（兩道各自會紅）。
- **不屬於單一寫入者的東西**：`contracts/`、`db/migrations/`、`db/queries/`、`db/query-owners.yaml` 與所有 generated 目錄由主 Agent 序列化。你的改動若需要動到其中任何一個（新 endpoint、新 query、schema 變更），停下來回報，不要自己改也不要繞過。
- **改完跑**：`go -C tools/devctl run . automation-check` ＋ 受影響套件的測試。**`go test ./...` 在沒有 `SKILLHUB_TEST_DATABASE_URL` 時是假綠**——integration 測試會 skip，那個 ok 是幾百條斷言拒絕執行。

**這個檔是給所有 coding agent 讀的**（Claude Code 讀不到 `AGENTS.md`，所以同目錄的 `CLAUDE.md` 只有一行 `@AGENTS.md` 把它 import 進來）。內容只寫在這一份，不複製。
