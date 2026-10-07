---
name: ci-by-rest
description: 想知道某個 commit 的 CI 有沒有過時使用。用 REST API 查而不是網頁，用完整 40 碼 SHA，列出該 SHA 的所有 workflow run（不要只挑主要那支），失敗時往下查出失敗的 job 與 step 名稱。
---

# 用 API 查 CI

## 先找 repo 自己的指令

先看 repo 有沒有把下面整套做成一支指令（例如 `ci-status [ref] [--wait]` 之類，或推送後自動盯 CI 的工具）。有就用它，下面的手動做法留給沒有這種指令的 repo，或指令本身壞掉的時候。

## 1. 沒有 `gh` 不等於查不到

曾經有一整個工作階段連續十幾次回報「CI 查不到（沒有 `gh`、沒有 token）」，而那句話從頭到尾是錯的——沒有人去測。先問：

```bash
curl -s https://api.github.com/repos/<owner>/<repo> | head -c 200
```

- `"private": false` → 公開 repo，**列 run、列 job 都不需要驗證**，`curl` 就夠；匿名每小時 60 次，夠用但別輪詢。
- HTTP 404 → 可能是私有，也可能是名字打錯：**未驗證時 GitHub 對私有 repo 回 404 而不是 403**。

## 2. 一律用完整 40 碼 SHA

短 SHA 查出來是零筆，而零筆長得像「還沒觸發」，不像「你查錯了」。用 `git rev-parse HEAD` 的完整值。

## 3. 列出該 SHA 的所有 run，不要用 workflow 名稱過濾

只查主要那支，另一支紅了你看不到。對 workflow runs 端點以 head SHA 過濾，不指定 workflow：

```bash
SHA=$(git rev-parse HEAD)
curl -s "https://api.github.com/repos/<owner>/<repo>/actions/runs?head_sha=$SHA" -o runs.json
node -e "for (const r of require('./runs.json').workflow_runs) console.log(r.name, r.status, r.conclusion, r.id)"
```

`total_count: 0` 的意思是「這個 SHA 沒有 run」：先確認 SHA 完整、已推上遠端，再說 CI 沒跑。

## 4. 失敗要查到 job 與 step

「CI 紅了」不是回報，是轉述。拿失敗 run 的 id 查 jobs 端點，報出失敗的 job 與 step 名稱：

```bash
curl -s ".../actions/runs/<run_id>/jobs?per_page=100" -o jobs.json
node -e "for (const j of require('./jobs.json').jobs) if (j.conclusion==='failure')
  console.log(j.name, '->', j.steps.filter(s=>s.conclusion==='failure').map(s=>s.name).join(' | '))"
```

## 5. 綠燈之前先問「它跑了嗎」

被條件跳過的 job 不會讓 run 變紅——只改了某一區時，另一區的 job 根本不存在，run 仍是 `success`。要確認某條紅線真的被驗過，在 jobs 清單裡確認那個 job 存在，不只看 run 的 conclusion。

## 6. 驗證是最後手段

列 run、列 job 不需要驗證；只有下載 job log 需要（公開 repo 也回 403）。先窮盡上面的匿名查詢與本機重現；真的需要 log 才用機器上已有的憑證（能 `git push` 就有，例如 `git credential fill`），而且**只放進變數當標頭用，永遠不印出、不寫檔、不放進回覆**。

## 回報格式

`<sha 前 7 碼>（以完整 SHA 查詢）：<n> 個 run — <workflow>: <conclusion>；…`

有失敗就附 `<job> / <step>`。
