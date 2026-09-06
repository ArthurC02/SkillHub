---
name: flowchart-to-skill-spec
description: 把流程圖忠實整理成中文的可執行技能需求說明；當你要把圖中的步驟、條件與分支轉成後續可撰寫 Skill 的文字時使用。
---

# Flowchart to Skill Spec

Use this skill when you need to turn a flowchart into a clear, executable skill specification in Chinese.

## Instructions

- Follow the flowchart exactly in the order shown.
- Include every node that appears in the flowchart.
- Include every condition that appears in the flowchart.
- Include every branch that appears in the flowchart.
- Do not add any branch, step, role, tool, or condition that the flowchart does not show.
- If the flowchart is silent about something, write **not given**.

## Required output

Produce a Chinese skill specification that preserves the flowchart's structure and meaning.

## Step 1: Read the flowchart nodes in order

List the nodes in the same order they appear in the flowchart.

## Step 2: State the condition

State the condition exactly as shown in the flowchart.

## Step 3: State the branches

State each branch exactly as shown in the flowchart, and keep the branch order from the diagram.

## Step 4: Omit nothing shown

Do not skip any node shown in the flowchart.

## Step 5: Do not invent anything

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape

Write the result in Chinese as a concise skill requirement document that can be used for later Skill authoring.

## Confirmed flowchart content to preserve

- 讀取伺服器 log 檔
- 篩出 ERROR 行
- 判斷錯誤是否超過 10 筆
- 如果是，建立 Jira 問題單
- 通知值班工程師
- 寫入每日摘要
- 如果否，直接寫入每日摘要
