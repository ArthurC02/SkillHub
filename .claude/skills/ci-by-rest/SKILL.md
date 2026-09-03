---
name: ci-by-rest
description: 想知道某個 commit 的 CI 有沒有過時使用。用 REST API 查而不是網頁，用完整 40 碼 SHA，列出該 SHA 的所有 workflow run（不要只挑主要那支），失敗時往下查出失敗的 job 與 step 名稱。
---

# 用 API 查 CI

## 1. 一律用完整 40 碼 SHA

短 SHA 查出來是**零筆**，而零筆長得像「還沒觸發」，不像「你查錯了」。這是這件事最常見的誤判。

    git rev-parse HEAD        # 或 git rev-parse <ref>

把完整那一串帶進查詢，不要截斷。

## 2. 列出該 SHA 的所有 run，不要用 workflow 名稱過濾

一個 repo 通常不只一個 workflow。只查主要那支，另一支紅了你看不到——那就是假綠燈。

請求形狀：對 repo 的 **workflow runs 端點**查詢，**以 head SHA 過濾**，不指定 workflow。例（GitHub）：

    gh api "repos/<owner>/<repo>/actions/runs?head_sha=$(git rev-parse HEAD)" \
      --jq '.workflow_runs[] | "\(.name)\t\(.status)\t\(.conclusion)"'

`total_count: 0` 的意思是「這個 SHA 沒有 run」。先確認 SHA 是完整的、而且已經推上遠端，再說 CI 沒跑。

## 3. 失敗要查到 job 與 step

「CI 紅了」不是回報，是轉述。拿失敗那個 run 的 id 去查 **jobs 端點**，報出失敗的 **job 名稱**與失敗的 **step 名稱**。

    gh api "repos/<owner>/<repo>/actions/runs/<run_id>/jobs" \
      --jq '.jobs[] | select(.conclusion=="failure")
            | "\(.name): \(.steps[] | select(.conclusion=="failure").name)"'

需要更多細節，再抓那個 job 的 log，不要一開始就抓。

## 4. 用 API，不要用網頁 UI

網頁是給人看的：會分頁、狀態有快取、也貼不回報告裡。私有 repo 兩邊都需要驗證（`gh auth status`，或帶 token 的 REST client）；**未驗證時 API 回 404 而不是 403**，看起來像 repo 不存在。

## 回報格式

`<sha 前 7 碼>（以完整 SHA 查詢）：<n> 個 run — <workflow>: <conclusion>；…`

有失敗就附 `<job> / <step>`。
