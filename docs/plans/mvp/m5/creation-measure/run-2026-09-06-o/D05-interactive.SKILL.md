---
name: new-hire-onboarding-flow-formatter
description: 將新人到職（onboarding）流程的流程圖或條列步驟整理成結構化、可直接使用的 Markdown 成果；當輸入包含新人到職流程時使用。
---

# New Hire Onboarding Flow Formatter

## 目的
把使用者提供的新人到職／onboarding 流程，整理成一份結構化、可直接使用的成果。

## 適用時機
當輸入明確是在描述新人到職流程、流程圖、條列步驟，或需要把這類流程改寫成內部說明稿時使用。

## 執行原則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 工作方式
1. 讀取使用者提供的新人到職流程內容。
2. 依照輸入中出現的流程節點，整理成清楚的 Markdown 結構。
3. 只保留輸入中已有的步驟與資訊；沒有提供的內容一律寫成 `not given`。
4. 不要新增流程分支、條件、角色、日期、工具或額外解釋。
5. 直接輸出完成品本身。

## 輸出要求
- 使用 Markdown。
- 保留流程順序。
- 必須包含輸入中出現的全部已確認節點。
- 若某項資訊在輸入中未提供，標示為 `not given`。
- 不得要求使用者補資料，除非輸入本身缺少完成任務所必需的內容。

## 已確認節點
若輸入提供的是本次已確認的新人到職流程，依序輸出以下節點：
1. 收到新人到職通知
2. 建立公司信箱
3. 加入 Slack 頻道
4. 準備筆電與門禁卡
5. 安排第一週訓練
6. 指派導師
7. 第 30 天面談
8. 記錄到人資系統

## 輸出格式
以 Markdown 列出流程步驟；若無額外資訊，僅輸出步驟清單即可。