# Draft PR: 讓版本停用引用接上主線 Domain Memory

## Business intent

管理後台版本停用與主線 Operations Context 合併後，讓已審查的 Registry→Run preflight 事實讀取仍能用現行程式碼驗證。

## Domain impact

`registry-run-preflight-facts` 的語意與 `registry-run-facts-v1` 契約不變。版本停用檢查讓同檔案的引用移動；主線審計鏈則已有三筆 Operations Context 事件。保留主線作為審計鏈基底，再經受控提案更新這一筆引用。

## Implementation handoff

只更新現有互動的結構化 citation，不改 Registry 擁有 Version 事實、Run 透過注入 reader 讀取的邊界。拒絕直接串接或重算既有雜湊事件，也不將舊分支的核准當作新版 Registry 的核准。

## Proposal and approvals

`run-preflight-version-disable-citation-mainline` 目前為 draft，尚無對目前 Registry revision 的 developer 核准。舊提案 `run-preflight-version-disable-citation` 的核准與套用紀錄保留在分支 Git 歷史，但不能直接移植。此提案需重新提交、驗證、核准與以簽章 SCM 證明套用。

## Contract impact

沿用 `registry-run-facts-v1`；不改公開或內部 API。

## Verification

已確認主線 67 筆審計事件鏈有效，原引用對合併後的 `preflight.go` 為唯一 stale citation。待新提案核准後，將以 `cite` 重算、`verify-evidence` 確認所有引用 current，並重跑 reader 缺席時拒絕的測試。

## Residual risks

本包只修復舊互動的證據引用；R-105、R-106 產品政策另行裁定，不因本包而視為核准。
