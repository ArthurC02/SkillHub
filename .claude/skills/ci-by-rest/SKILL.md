---
name: ci-by-rest
description: 想知道某個 commit 的 CI 有沒有過時使用。用 REST API 查而不是網頁，用完整 40 碼 SHA，列出該 SHA 的所有 workflow run（不要只挑主要那支），失敗時往下查出失敗的 job 與 step 名稱。
---

# 用 API 查 CI

## 0. 沒有 `gh` 不等於查不到

**這一節是這份技能最貴的一課**：2026-09-09 有一整個工作階段連續十幾次回報「CI 查不到（沒有 `gh`、沒有 token）」，而那句話**從頭到尾是錯的**。沒有人去測，只是看到 `gh` 不在 PATH 上就結案了。

先花三十秒問這三個問題，順序不要換：

```bash
command -v gh || echo "no gh"            # 沒有也不影響下面兩步
command -v curl                           # Git for Windows 自帶，幾乎一定有
curl -s https://api.github.com/repos/<owner>/<repo> | head -c 200
```

第三步的回應決定一切：

- **`"private": false`** → 這個 repo 是公開的，**列 run、列 job 完全不需要驗證**，`curl` 就夠。這正是本專案的情況。
- HTTP 404 → 可能是私有，也可能是名字打錯。**未驗證時 GitHub 對私有 repo 回 404 而不是 403**，所以 404 不能當成「不存在」。

## 0.1 需要驗證時，憑證通常已經在機器上

**能 `git push` 就代表有憑證。** Windows 用 Git Credential Manager，取出來不必問任何人：

```bash
TOKEN=$(printf 'protocol=https\nhost=github.com\n\n' | git credential fill | sed -n 's/^password=//p')
curl -s -H "Authorization: Bearer $TOKEN" https://api.github.com/...
```

**永遠不要把它印出來、寫進檔案、或放進回覆**（鐵律 11）。只放進變數，只當成標頭用。想確認拿到了就印長度，不要印值。

需要驗證的只有**下載 job log**（公開 repo 也一樣回 403）；列 run 與列 job 不需要。

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

**沒有 `gh` 的等價寫法**（`curl` ＋ `node -e`，本專案兩者都有）：

```bash
SHA=$(git rev-parse HEAD)
curl -s "https://api.github.com/repos/<owner>/<repo>/actions/runs?head_sha=$SHA" -o runs.json
node -e "for (const r of require('./runs.json').workflow_runs) console.log(r.name, r.status, r.conclusion, r.id)"
curl -s ".../actions/runs/<run_id>/jobs?per_page=100" -o jobs.json
node -e "for (const j of require('./jobs.json').jobs) if (j.conclusion==='failure')
  console.log(j.name, '->', j.steps.filter(s=>s.conclusion==='failure').map(s=>s.name).join(' | '))"
# log 是唯一需要驗證的一步（見 §0.1），而且要 -L：它會轉址到一個簽章過的網址
curl -sL -H \"Authorization: Bearer \$TOKEN\" ".../actions/jobs/<job_id>/logs" -o job.log
```

## 3.1 綠燈之前先問「它跑了嗎」

一個**被跳過**的 job 不會讓 run 變紅。本專案的 `web-browser` 有 `if: needs.changes.outputs.web == 'true'`，所以沒動 `apps/web` 的 commit 上它根本不存在——而那個 run 仍然是 `success`。

**所以「這個 SHA 是綠的」不等於「該跑的都跑過了」。** 判斷某一條紅線有沒有真的被驗過，要在 jobs 清單裡確認那個 job **存在**，不只是看 run 的 conclusion：

```bash
node -e "const j=require('./jobs.json').jobs.find(x=>x.name.includes('<job 名稱片段>'));
  console.log(j ? j.conclusion : 'NOT PRESENT — 這個 SHA 沒跑過它')"
```

## 4. 用 API，不要用網頁 UI

網頁是給人看的：會分頁、狀態有快取、也貼不回報告裡。私有 repo 兩邊都需要驗證（`gh auth status`，或帶 token 的 REST client）；**未驗證時 API 回 404 而不是 403**，看起來像 repo 不存在。

## 回報格式

`<sha 前 7 碼>（以完整 SHA 查詢）：<n> 個 run — <workflow>: <conclusion>；…`

有失敗就附 `<job> / <step>`。
