---
type: Capability
status: draft
sources:
  - resource: "db/queries/run_continuations.sql#L59-123"
    excerpt_sha256: "sha256:a5fb0e5a960b2138d418de22ac6554c3e347938b98fc20d7311a0e51ccd062df"
  - resource: "db/migrations/0152_runs_created_at_index.sql#L1-1"
    excerpt_sha256: "sha256:90ebe0f888320c32bc42acd9bcc338ffce4cb24c848a91366a1664beb24527a7"
  - resource: "apps/platform/internal/trial/execution/supervisor.go#L207-225"
    excerpt_sha256: "sha256:878c15cbd9b57ab8b5a1ede44292a7dcf1abc9279d3fa66a0ffd677d71b46527"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/continuation_metrics_integration_test.go#L1-265"
    excerpt_sha256: "sha256:02d84ead0f0bf934d849725540efc203b86f1260cefeb9a24433f017a654917a"
---
觀察仍保留的 Run 接續鏈：以最近 24 小時建立的初始 Run 為同一批，計鏈數、接續輪數總和與最長輪數，包含所有狀態、評估與不載入 Skill 的對照。初始 Run 本身為零輪；中間 Run 不另計一條鏈。這是觀察當下已保存的結構，不能當成人類對話、完整對話或採用率。讀取失敗撤下數字與觀察時間，成功讀到空集合才顯示零；各 Worker 副本不可相加。不讀問答原文，不新增私人資料保存或動態標籤，亦不影響授權、派送、成本或帳號清除政策。

# Relations
- Context: [run](/contexts/run)
