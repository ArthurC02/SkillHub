# Domain Memory 治理

`review_governance` 是團隊選定的審核證據，不是自動偵測結果。探索得到的 CI 只供選擇；`ci_requirement` 可為 `required`、`optional` 或 `none`。

沒有 CI 時，選擇 `git-signed-commit`、`git-push` 與至少一個授權簽署者。執行 `install-git-hitl-hook` 後，pre-push 會驗證推送中每個異動 Domain Memory 的 commit。既有 pre-push hook 會保留為 `pre-push.domain-memory-existing` 並先執行。

本機 hook 可被 `--no-verify` 或未安裝 hook 的機器略過，不能作為唯一信任邊界；正式保護仍須由受保護分支、伺服器端 hook 或選定的 SCM 治理執行。Git 簽章可使用 GPG 指紋或 Git 回報的 SSH signer identity，但 policy 必須列出其授權值。
