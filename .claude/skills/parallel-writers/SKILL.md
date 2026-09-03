---
name: parallel-writers
description: Use when more than one agent or person is editing the same working tree at the same time, or before staging/committing while someone else has uncommitted work in the tree.
---

# Parallel Writers

多個 writer 共用同一份 working tree 時，別人的未提交／未追蹤變更跟你的變更混在同一個目錄。以下規則的目的是讓你的動作只影響自己的檔案。

## 規則

1. **只用明確 pathspec stage 自己的檔案。** 絕不用 `git add -A` 或 `git add .`——那會連同別人未提交、未追蹤的工作一起收進 stage。逐一列出檔名（或用精確的目錄/glob），只列你自己改的。

2. **不是你造成的 delta，一律視為不可碰。** 看到工作樹裡有你沒改過的變更（不論已追蹤或未追蹤），不要 revert、不要 stash、不要 reset、不要 checkout 掉它。留著它，回報給協調者或使用者，讓它自己決定。

3. **`git stash` 是這種情境下最危險的指令，禁止使用。** stash 會把整個工作樹（包含別人的未提交與未追蹤檔案）一起收走，而且收走的過程不會提示「這不是你的檔案」——遺失是無聲的，事後很難察覺。

4. **push 前先對遠端 rebase。** 如果 rebase 因為別人有未 stage 的變更而失敗，不要為了讓它跑起來就清空工作樹（不准 `reset --hard`、不准 `clean -f`、不准 `checkout .`）。要嘛改成 fast-forward push，要嘛先等對方 commit 或收工再重試。

5. **依檔案切分 writer 責任，不要讓兩個 writer 同時擁有同一個檔案。** 測試檔、設定檔、生成目錄這類天生共享的檔案，指定給一個協調者統一處理，其他 writer 不碰。

6. **commit 前把「看一眼索引」當成獨立的一步，不要和 commit 串在同一條指令上。** 用明確 pathspec `add` 並不保證索引裡只有你的檔案——索引可能在你上次查看之後被別人加過東西。所以要單獨執行一次 `git diff --name-only --staged`，**讀完結果再決定要不要 commit**。如果寫成 `git add X && git diff --staged && git commit`，那份清單會照樣印出來，但它是在 commit 已經跟著執行之後才被你看到——**串在一起的檢查不是檢查，只是輸出**。

7. **別人區域裡的失敗，預期會發生，忽略它。** 你沒有負責的檔案裡如果測試紅了，通常是別人正在進行中的工作，不是你造成的 bug；不要去「順手修好」，那會製造衝突與越界修改。

## 快速檢查（動手前）

- 我要 stage 的每個檔案，都是我自己改的嗎？
- 工作樹裡還有其他人的 delta 嗎？我打算怎麼處理——留著，還是清掉？如果是清掉，停下來。
- 我的 commit 是獨立的一步，還是和檢查串在同一條 `&&` 鏈上？
- 我要下的指令裡有 `stash`／`reset`／`clean`／`checkout --` 嗎？如果有，先確認目標只涉及我自己的檔案。
