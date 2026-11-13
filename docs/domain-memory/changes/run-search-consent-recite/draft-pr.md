# 更新 Registry 到 Run 的版本事實 reader 引用

Run 從指定不可變版本取得 Compatibility，據此列出搜尋需求、確認權限摘要與保存政策。這份提案僅更新既有 internal-fact-reader v1 的程式引用；Registry 生產者、Run 消費者、同步讀取方式、內部分類與「消費端事實刻意演進」的相容政策都維持既有內容。

引用仍涵蓋完整的版本事實 reader 與映射函式。沒有新增 Context 依賴、狀態寫入、事件或公開 API；搜尋授權決策仍由 Run 持有。版本宣告與授權行為由平台單元測試、PostgreSQL 流程測試及反證驗證，來源引用由本提案的 evidence-current 義務核對。

原生工具會依例行引用更新流程提交、記錄驗證並核對簽署 Commit，沿用未改變事實的既有審核。沒有新增人工同意或替其他待審提案放行。runtime 的實際能力讀取、搜尋工具、私人 Trace 與真實品質仍待其他交付。
