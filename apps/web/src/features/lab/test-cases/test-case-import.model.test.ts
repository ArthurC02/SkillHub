import { expect, test } from "vitest";
import { parseTestCaseFile } from "./test-case-import.model";

test("each JSONL object becomes one row, query stands in for user_prompt and a missing name is numbered", () => {
  const text = [
    '{"name":"缺日期","user_prompt":"  算總額  ","criteria":["指出缺日期"," ","指出缺日期"]}',
    "",
    '{"query":"只有 query"}',
  ].join("\n");
  expect(parseTestCaseFile("cases.JSONL", text)).toEqual({
    rows: [
      { line: 1, name: "缺日期", user_prompt: "算總額", criteria: ["指出缺日期"] },
      { line: 3, name: "匯入的第 3 列", user_prompt: "只有 query", criteria: [] },
    ],
    problems: [],
  });
});

test("JSONL lines that are not an object, not JSON, or have no prompt are reported by line", () => {
  const text = [
    '["a"]',
    "{oops",
    '{"name":"空的","user_prompt":"  "}',
    '{"user_prompt":"p","criteria":3}',
  ].join("\n");
  expect(parseTestCaseFile("a.jsonl", text)).toEqual({
    rows: [],
    problems: [
      { line: 1, reason: "每一行要是一個 JSON 物件。" },
      { line: 2, reason: "這一行不是有效的 JSON。" },
      { line: 3, reason: "缺少 user_prompt（或 query），這一題沒有可以跑的提示。" },
      { line: 4, reason: "criteria 要是一串文字。" },
    ],
  });
});

test("a CSV quoted field keeps commas, doubled quotes and newlines, and later rows keep their own line", () => {
  const text =
    '﻿Name,User_Prompt,criteria\r\n"a, b","說 ""你好""","第一條\n第二條"\r\n\r\nc,第二題,\n';
  expect(parseTestCaseFile("cases.csv", text)).toEqual({
    rows: [
      { line: 2, name: "a, b", user_prompt: '說 "你好"', criteria: ["第一條", "第二條"] },
      { line: 5, name: "c", user_prompt: "第二題", criteria: [] },
    ],
    problems: [],
  });
});

test("a CSV without a prompt column, or with an unclosed quote, is refused as a whole", () => {
  expect(parseTestCaseFile("a.csv", "name,answer\nx,y\n").problems).toEqual([
    { line: 1, reason: "第一列要是欄位名稱，而且要有 user_prompt（或 query）欄。" },
  ]);
  expect(parseTestCaseFile("a.csv", 'query\n"open\n').problems).toEqual([
    { line: 0, reason: "第 2 列的引號沒有成對，檔案沒有讀。" },
  ]);
});

test("another file type is refused", () => {
  expect(parseTestCaseFile("cases.json", "{}")).toEqual({
    rows: [],
    problems: [{ line: 0, reason: "只能匯入 .jsonl 或 .csv 檔。" }],
  });
});

test("50 rows are read and the 51st stops the import with a note", () => {
  const lines = Array.from({ length: 52 }, (_, i) => JSON.stringify({ query: `題 ${i + 1}` }));
  const parsed = parseTestCaseFile("a.jsonl", lines.join("\n"));
  expect(parsed.rows).toHaveLength(50);
  expect(parsed.rows[49].user_prompt).toBe("題 50");
  expect(parsed.problems).toEqual([
    { line: 51, reason: "一次最多匯入 50 題，從這一列起都沒有讀。" },
  ]);
});

test("limits are taken at the edge: 200-character name, 2000-character criterion, 20 criteria", () => {
  const row = (fields: object) =>
    parseTestCaseFile("a.jsonl", JSON.stringify({ query: "p", ...fields }));
  expect(row({ name: "名".repeat(200) }).rows).toHaveLength(1);
  expect(row({ name: "名".repeat(201) }).problems[0].reason).toBe(
    "名稱最多 200 個字，目前 201 個字。",
  );
  expect(row({ criteria: ["條".repeat(2000)] }).rows).toHaveLength(1);
  expect(row({ criteria: ["條".repeat(2001)] }).problems[0].reason).toBe(
    "每條驗收條件最多 2000 個字，目前 2001 個字。",
  );
  const many = (n: number) => Array.from({ length: n }, (_, i) => `條件 ${i}`);
  expect(row({ criteria: many(20) }).rows[0].criteria).toHaveLength(20);
  expect(row({ criteria: many(21) }).problems[0].reason).toBe(
    "驗收條件最多 20 條，這一列有 21 條。",
  );
});

test("a prompt one character past its limit is refused and one at it is kept", () => {
  const at = parseTestCaseFile("a.jsonl", JSON.stringify({ query: "字".repeat(32768) }));
  const past = parseTestCaseFile("a.jsonl", JSON.stringify({ query: "字".repeat(32769) }));
  expect(at.rows).toHaveLength(1);
  expect(past.problems[0].reason).toBe("Prompt 最多 32768 個字，目前 32769 個字。");
});
