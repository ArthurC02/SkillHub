# 受限機器的環境探測：六個問題，十五分鐘

- 日期：2026-08-29
- 為什麼存在：M6 的每一條路都掛在**同一個沒有人去問的事實**上——那台機器到底准跑什麼。[`m6/README`](README.md) 的啟動條件 2 從 2026-08-28 起就是「未取得」。
- 誰跑：**任何一個能坐在那台機器前面的人。** 不需要開發環境、不需要管理員、不安裝任何東西；除了 Q6 在暫存目錄建一個虛擬環境、量完即刪，全部唯讀。
- 結果回填到：[`m6/README`](README.md) 啟動條件 2 與 [淨測試模式](../../../adr/README.md#淨測試模式)的待決策。

> **這份清單不做任何繞過。** 每一項都是「問系統它允不允許」，不是「試著讓它允許」。任何一項失敗都是有價值的答案，**不要想辦法讓它過**。

## Q1（最重要）：Node 在不在白名單上

**為什麼問**：資料庫那一軸的**兩條候選路都跑在 Node 裡**（PGlite 與 pgmock 都是 npm 套件）。**這一題是「否」的話，整個 M6 的資料庫那一半要重新設計。**

```powershell
node --version
npm --version
```

| 結果 | 意思 |
| --- | --- |
| 印出版本 | ✅ 兩條路都還活著 |
| `不是內部或外部命令` | Node 沒裝——**要問的是「能不能裝」，而不是「能不能跑」**，見 Q2 |
| 被封鎖／權限錯誤 | ❌ **最壞的答案**，而且是最需要知道的那個 |

## Q2：使用者資料夾裡的未簽章執行檔，跑不跑得起來

**為什麼問**：這一題**同時**回答「能不能自己帶東西上去」與「Go 的測試跑不跑得動」。Go 會把編譯出來的**未簽章執行檔丟進 `%TEMP%` 然後執行它**——那在 AppLocker 眼中，和一顆解壓出來的 `postgres.exe` 是同一類東西。

```powershell
go version
cd <這個 repo>\apps\platform
go test ./internal/skill/library/
```

| 結果 | 意思 |
| --- | --- |
| 測試跑起來（不論綠紅） | ✅ **白名單沒有在攔使用者目錄的未簽章執行檔**——前面所有關於「帶不上去」的擔心多半是多餘的 |
| 被封鎖 | ❌ 那麼「在這台機器上跑測試」從一開始就不成立，**與資料庫無關** |

## Q3：實際生效的政策是什麼

**全部唯讀，不需要管理員。**

```powershell
Get-AppLockerPolicy -Effective -Xml
citool.exe -lp
Get-WinEvent -LogName "Microsoft-Windows-AppLocker/EXE and DLL" -MaxEvents 20
```

**要看的是兩件事**：有沒有規則存在，以及 `EnforcementMode` 是 `Enabled` 還是 `AuditOnly`。**很多機構第一階段只開稽核**——若是 `AuditOnly`，那「被擋」與「被記錄」是兩回事。

## Q4：Edge 開得開本機 HTML 嗎

**為什麼問**：`02:PORT-005` 要求前端在 Edge 上驗過。**實測過的界線是「不能 `import`」不是「不能用 module」**（[report-inmemory-postgres.md](report-inmemory-postgres.md) §9）。<br>**注意這一題的權重 2026-08-29 降了**：[淨測試模式](../../../adr/README.md#淨測試模式)之後前端是由 localhost 上的服務供應，不是 `file://` 開的單檔，所以「否」不再讓交付形式重新設計——但它仍然是唯一能事先知道企業瀏覽器政策有多嚴的一題。

把下面存成 `probe.html` 放桌面，用 Edge 打開：

```html
<!doctype html><html><body><div id="out">NOT RUN</div>
<script>document.getElementById("out").textContent = "classic ok";</script>
<script type="module">
  document.getElementById("out").textContent += " | inline module ok";
  const w = await WebAssembly.instantiate(
    Uint8Array.from(atob("AGFzbQEAAAABBQFgAAF/AwIBAAcHAQNydW4AAAoGAQQAQSoL"), c => c.charCodeAt(0)), {});
  document.getElementById("out").textContent += " | wasm ok -> " + w.instance.exports.run();
</script></body></html>
```

**畫面上應該出現**：`classic ok | inline module ok | wasm ok -> 42`

| 結果 | 意思 |
| --- | --- |
| 三段都出現 | ✅ 交付形式可行（**前提是全部內嵌成單檔**） |
| 停在 `NOT RUN` 或少一段 | ⚠️ 企業瀏覽器政策比預設更嚴。**不擋 `PORT-005`**（前端由 localhost 供應），但要在該機器上重驗一次前端 |

## Q5：對外到底通得到哪裡

**為什麼問**：`05` R-23 的建議是「先問防火牆能不能多開一個網域」——**在問之前要先知道現在通得到什麼。**

```powershell
foreach ($h in "api.openai.com","github.com","registry.npmjs.org","proxy.golang.org") {
  $r = Test-NetConnection -ComputerName $h -Port 443 -WarningAction SilentlyContinue
  "{0,-24} {1}" -f $h, $(if ($r.TcpTestSucceeded) { "OK" } else { "BLOCKED" })
}
```

**`registry.npmjs.org` 與 `proxy.golang.org` 特別重要**：不通的話，連把相依帶上去都得另想辦法（離線打包），那是一項**本 repo 目前完全沒有規劃過**的工作。

## Q6：Python 與 uv 跑不跑得起來，能不能建虛擬環境

**為什麼問**：跨語言意圖搜尋、評估判定、匯入增強、推薦理由四項能力都住在 `apps/llm`，平台一律經它呼叫模型，沒有繞過去直連閘道的路。它是 Python ＋ uv ＋ FastAPI，要求 `apps/llm/pyproject.toml` 的 `requires-python`。**Q1～Q5 沒有一題問到 Python**；啟動指令碼 `tools/cleanmode/start.mjs` 也不起 `apps/llm`，離線打包沒有 Python 那一半。這一題是「否」的話，demo 上這四項能力都不會出現。

```powershell
python --version
py --version
uv --version
python -m venv "$env:TEMP\skillhub-probe-venv"
& "$env:TEMP\skillhub-probe-venv\Scripts\python.exe" -c "print('venv ok')"
Remove-Item -Recurse -Force "$env:TEMP\skillhub-probe-venv"
```

| 結果 | 意思 |
| --- | --- |
| 版本符合 `requires-python`，而且印出 `venv ok` | ✅ `apps/llm` 在那台機器上有執行期；接著要問的是 uv 能不能裝或帶上去 |
| `python` 有，但版本低於 `requires-python` | ⚠️ 要問的是「能不能裝新版」，不是「能不能跑」 |
| `python` 或 `uv` 是 `不是內部或外部命令` | 沒裝——同 Q1，要問的是能不能裝 |
| 建虛擬環境或執行 `Scripts\python.exe` 被封鎖 | ❌ `apps/llm` 在那台機器上沒有執行期。退路是播放真實錄下的 Run，或以本機 Driver 現跑策展素材 |

## 回填

| 問題 | 答案 | 誰答的 | 日期 |
| --- | --- | --- | --- |
| Q1 Node | ✅ **可以跑** | 產品負責人 | 2026-08-29 |
| Q2 未簽章執行檔 | ✅ **可以執行 Go** | 產品負責人 | 2026-08-29 |
| Q3 政策模式 | | | |
| Q4 Edge 本機檔案 | | | |
| Q5 對外連線 | ⚠️ **不是 Q5 指令的輸出，是負責人答覆 [`05` R-23](../../05-pending-rulings.md) 的實測**：防火牆不能多開網域；前後端所需套件都取得得到；OpenAI API 打得通；網頁搜尋大部分可達。是實測不是保證，見 [README](README.md)「環境的三個事實」 | 產品負責人 | 2026-08-29 |
| Q6 Python／uv | | | |

~~**Q1 是唯一一題會改變設計的。**~~ **2026-08-29：Q1 與 Q2 都答了，都是「可以」。**

**A 半（淨測試模式）因此在那台機器上成立**：資料庫那一軸的 PGlite 是 Node 套件（Q1），啟動指令碼現場建的 `cmd/api` 與 `sandboxd` 是 Go 程式（Q2），離線相依兩半都已打包並驗過。

**B 半（在那台機器上跑 Go 測試套件）仍然不成立，而擋住它的東西換了**——不再是環境，是程式。同日對真的 PGlite 承載試過一次（暫時繞過測試守衛、量完即還原）：

| 走到哪 | 結果 |
| --- | --- |
| 測試守衛 | 擋在資料庫名稱不以 `_test` 結尾。**PGlite 只有一個叫 `postgres` 的資料庫**，而它在連線時接受任何名稱——**所以用一個假名字就能過關，而那正是本 repo 最不該用的通過方式** |
| 連線池 | 預設開多條連線，承載只收一條，**第一次就把承載弄卡** |
| 加上 `pool_max_conns=1` | **死鎖，十分鐘不動** |

**死鎖的原因文件早就寫著**：`lockTestSchema` 握著一條連線再去開第二條，而有些測試（advisory lock 序列化、並行首次登入）**本質上就需要多於一條連線**。

**所以 B 半等的是一次程式改動，不是一個答案**，而 `02` §4.10 明文寫著它「本節不主張」。要開它需要先裁定。

**而答案一到手，擋著的那一題就換人了。** Q1 回答的是「PGlite 跑不跑得起來」；但今天的啟動指令碼（`tools/cleanmode/start.mjs`）**還需要 `go`**——`cmd/api` 與 `sandboxd` 是 Go 程式，指令碼是在那台機器上現場建它們。

**所以現在的關鍵路徑是 Q2**，而 Q2 問的正是這件事：Go 會把編譯出來的**未簽章執行檔丟進 `%TEMP%` 然後執行它**，那在 AppLocker 眼中和一顆解壓出來的執行檔是同一類東西。

| Q2 的答案 | 意思 |
| --- | --- |
| 測試跑得起來 | ✅ **整個模式在那台機器上成立**，剩下的只有相依要離線帶上去 |
| 被封鎖 | ❌ **今天的啟動形式在那台機器上跑不了**。替代路只有「事先建好的二進位帶上去」，而那是同一道白名單問題的另一種問法——**不是繞過，是同一堵牆的另一面** |

**Q3／Q4／Q5 仍然不擋動工**：Q3 分辨「被擋」與「只被記錄」，Q4 決定前端要不要在該機器上重驗，Q5 決定相依離線打包要做到多徹底。

**Q6 不擋淨測試模式本身，擋的是 demo 上的四項模型能力**：答「否」時改走播放真實錄下的 Run 或本機 Driver 現跑策展素材，不是讓那四項能力在畫面上壞掉。
