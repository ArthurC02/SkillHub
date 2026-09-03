# 功法：換一個 repo 也成立的具體手法

每一條都在本歷程裡至少用過一次，且有一次因為沒用而出事。條目格式：手法 → 什麼時候用 → 怎麼做 → 出過什麼事。

## 探測要帶唯一標記，而且要有對照組

**用在**：驗證任何「這個機制會不會把文字送到 agent 面前」的問題。
**做法**：建一個內容含唯一字串（如 `PROBE-7f3a`）的探測檔放進候選機制，派子代理讀會命中的檔案並要求回報「有沒有看到那個字串」。同時派一個讀不會命中之檔案的對照組。兩個都回來才下結論。
**出過的事**：`docs.md`／`serialized-areas.md` 兩條 rule 被判「不會跳」，其實是探測檔沒命中 glob——沒有對照組，就分不清「機制無效」和「我探錯了」。

## deny 要撞，撞完要對照

**用在**：任何 `permissions.deny`、hook、lint 規則。
**做法**：乾淨工作樹上執行會被擋的指令（能 `--dry-run` 就加）；再執行結構相同但合法的（`git add <path>`、`git commit --author=`）確認通過；再派子代理撞一次。
**出過的事**：`Bash(*git add .*)` 寫完幾分鐘內誤擋了 `git add .claude/…`；`commit -a`、`restore`、`checkout .` 在審視前都漏擋。

## 檢查是獨立的一步，不串在動作前面

**用在**：commit、push、刪除、任何不可逆動作前的確認。
**做法**：`git diff --name-only --staged` 單獨執行、讀完結果、再下 `git commit`。不寫 `check && act`。
**出過的事**：`git add X && git diff --staged && git commit` 把別人 25 個檔掃進 commit——清單印出來時 commit 已經跑完。

## 新機器先種違規，看它紅

**用在**：每一條新 checker、新測試、新 CI 步驟。
**做法**：對真樹跑一次確認綠；手動造一個最小違規（拔掉一個 `name:`、加一行 `docs/`、刪掉 `model:`）；跑；看到 FAIL 且訊息指到對的檔案行號；還原；`git diff -- <file>` 為空。
**出過的事**：檢查器第一版對 `ADR-034` 命中兩次（兩條 regex 重疊），是種違規時測試抓到的，不是我看程式碼看到的。

## 拿產品自己的驗證器驗 harness

**用在**：repo 的產品本身有驗證某種格式的能力時（本 repo：Agent Skills 套件驗證器）。
**做法**：一支測試把驗證器跑在 harness 的對應目錄上，只看 error 等級。不重寫規則、不複製 regex。
**出過的事**：五個技能有兩個沒 `name:`，Claude Code 不在乎（用目錄名），產品的驗證器判 `name-missing`。

## 儀器優先於推論

**用在**：「有沒有送達」「有沒有觸發」這類問題。
**做法**：一個 `InstructionsLoaded` hook（Claude Code）把 stdin 的 JSON 壓成一行附時間戳寫進 scratchpad 的 log；問題出現時先 `grep` log，不先想。
**出過的事**：說「儀器從沒觸發」時 log 已有 16 列。

## 通過的定義是看到成功那一行

**用在**：任何被 pipe、代理層或 token 過濾器（本環境的 rtk）截斷過的命令輸出。
**做法**：先說出這個工具成功時會印什麼（`ok <pkg>`、`All matched files use Prettier code style!`），然後在輸出裡找到它；找不到就 `> out.txt 2>&1; echo exit=$?` 再讀檔。
**出過的事**：`prettier --check` 只印 `Checking formatting...` 就被截掉，exit 其實是 1。

## 用 API 查 CI，用完整 SHA，列全部 run

**用在**：判斷某個 commit 的 CI。
**做法**：`git credential fill` 取 token → `GET /repos/<o>/<r>/actions/runs?head_sha=<40 碼>` → 等所有 run `completed` → 逐一報 `name`／`conclusion`。背景輪詢，不用網頁。
**出過的事**：短 SHA 查出零筆，長得像「還沒觸發」；`cancel-in-progress` 讓前一筆顯示 `cancelled`，要知道那是被後一筆頂掉，不是失敗。

## 多行內容不走 heredoc，走檔案工具

**用在**：寫含反引號、`$`、反斜線、BOM 的原始碼或 markdown。
**做法**：用編輯器級的 Write／Edit 工具；bash 只跑指令。sed 遇到反引號模式時改 Edit。
**出過的事**：一個 heredoc 因反引號整段 parse 失敗（什麼都沒寫）；sed 把 `﻿` 吃成 `FEFF`；Go 拒絕檔案中間出現字面 BOM。

## 記憶檔只記「換一個 session 還需要」的事

**用在**：session 記憶。
**做法**：一檔一事，附 Why／How to apply；索引一行。repo 記得的（結構、歷史）不記；對人的偏好（回覆格式、模型階梯）記在這裡而不是 repo。
**出過的事**：模型階梯第一版寫在 `.claude/`，Codex 讀不到；改成規則進 repo、偏好進記憶。

## 審視要對照路線圖，不對照現況

**用在**：任何「這個設計好不好」的問題。
**做法**：打開路線圖，寫下下一階段的工作會落在哪些目錄、哪些任務類型；逐層問「蓋得到嗎」。蓋不到又要新增結構（角色、上限、名字）的，就是借了現況的便宜。
**出過的事**：按目錄切的兩個角色、抄五份的禁令、28 KiB 的上限。

## 回覆三段，每段 2～5 點

**用在**：每一次回報。
**做法**：執行結果／待辦清單／需要確認；子代理的回報自己消化，不轉貼；表格只在數字會改變決定時用；白話段兩三句。
**出過的事**：負責人明言「冗長、重點不多、非常困擾」。
