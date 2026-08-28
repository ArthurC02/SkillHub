# 受限機器的環境探測：五個問題，十分鐘

- 日期：2026-08-29
- 為什麼存在：M6 的每一條路都掛在**同一個沒有人去問的事實**上——那台機器到底准跑什麼。[`m6/README`](README.md) 的啟動條件 2 從 2026-08-28 起就是「未取得」。
- 誰跑：**任何一個能坐在那台機器前面的人。** 不需要開發環境、不需要管理員、全部唯讀，不安裝任何東西。
- 結果回填到：[`m6/README`](README.md) 啟動條件 2 與 [ADR-058](../../../adr/ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md) 的待決策。

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

**為什麼問**：`02:PORT-005` 的交付形式掛在這上面。**實測過的界線是「不能 `import`」不是「不能用 module」**（[report-inmemory-postgres.md](report-inmemory-postgres.md) §9）。

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
| 停在 `NOT RUN` 或少一段 | ❌ 企業瀏覽器政策比預設更嚴，`PORT-005` 要重新設計 |

## Q5：對外到底通得到哪裡

**為什麼問**：`05` R-23 的建議是「先問防火牆能不能多開一個網域」——**在問之前要先知道現在通得到什麼。**

```powershell
foreach ($h in "api.openai.com","github.com","registry.npmjs.org","proxy.golang.org") {
  $r = Test-NetConnection -ComputerName $h -Port 443 -WarningAction SilentlyContinue
  "{0,-24} {1}" -f $h, $(if ($r.TcpTestSucceeded) { "OK" } else { "BLOCKED" })
}
```

**`registry.npmjs.org` 與 `proxy.golang.org` 特別重要**：不通的話，連把相依帶上去都得另想辦法（離線打包），那是一項**本 repo 目前完全沒有規劃過**的工作。

## 回填

| 問題 | 答案 | 誰答的 | 日期 |
| --- | --- | --- | --- |
| Q1 Node | | | |
| Q2 未簽章執行檔 | | | |
| Q3 政策模式 | | | |
| Q4 Edge 本機檔案 | | | |
| Q5 對外連線 | | | |

**答完之後才動工。** 目前 M6 的九項未完工作項裡，有七項的形狀取決於 Q1 與 Q4 的答案——**先寫的風險是白寫**。
