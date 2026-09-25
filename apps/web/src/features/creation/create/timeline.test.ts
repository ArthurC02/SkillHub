import { expect, test } from "vitest";
import { buildRoundTimeline } from "./create.model";

const question =
  "這次試跑有條件沒過：\n- 「明確統計」：沒過——未列出有效數字列數\n要照這些條件改草稿、還是改條件或範例輸入？也可以直接說你要它改哪裡。";
const trial = {
  role: "tool" as const,
  content:
    '{"evaluation":{"evaluation_available":true,"overall":"partially_met","criterion_results":[{"result":"passed"},{"result":"failed"}]}}',
};

test("the timeline keeps the criterion and reason behind a follow-up question", () => {
  expect(buildRoundTimeline([{ role: "assistant", content: question }])).toEqual([
    { key: "t-0", text: `系統問你：${question}` },
  ]);
});

test.each([
  ["validation", '{"validation":{"valid":true}}'],
  ["unreadable tool result", "{"],
  ["fetch", '{"fetch":{"url":"https://example.com","status":"ok"}}'],
])("%s messages do not sever the question, answer and revision", (_, content) => {
  const timeline = buildRoundTimeline([
    trial,
    { role: "assistant", content: question },
    { role: "tool", content },
    { role: "user", content: "請保留三個統計欄位" },
    { role: "tool", content },
    { role: "assistant", content: "已補上總列數、有效數字列數與缺值列數" },
    { role: "assistant", content: "其他對話" },
    { role: "user", content: "另一個問題" },
  ]);
  expect(timeline.filter((item) => !item.text.startsWith("讀取網頁："))).toEqual([
    { key: "t-0", text: "第 1 次試跑：部分達成（通過 1／不通過 1／無法判定 0）" },
    { key: "t-1", text: `系統問你：${question}` },
    { key: "t-3", text: "你回答：請保留三個統計欄位" },
    { key: "t-5", text: "模型建議：已補上總列數、有效數字列數與缺值列數" },
  ]);
});

test("a new trial stops waiting for the previous trial's answer", () => {
  expect(
    buildRoundTimeline([
      { role: "assistant", content: question },
      trial,
      { role: "user", content: "另一個問題" },
      { role: "assistant", content: "另一個回答" },
    ]),
  ).toEqual([
    { key: "t-0", text: `系統問你：${question}` },
    { key: "t-1", text: "第 1 次試跑：部分達成（通過 1／不通過 1／無法判定 0）" },
  ]);
});
