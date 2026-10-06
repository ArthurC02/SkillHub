import { expect, test } from "vitest";
import { serverSentenceOr } from "./format";

test.each([
  ["請重新確認後再試。", "請重新確認後再試。"],
  ["Skill Hub 還在封測，請先申請。", "Skill Hub 還在封測，請先申請。"],
  ["Bundle 需要一段說明。", "Bundle 需要一段說明。"],
  ["summary_hash does not match", "操作未完成，請重新確認後再試。"],
  ["internal error: 儲存失敗", "操作未完成，請重新確認後再試。"],
  ["Internal Error 儲存失敗", "操作未完成，請重新確認後再試。"],
  [undefined, "操作未完成，請重新確認後再試。"],
])("server sentence %s has a readable result", (message, expected) => {
  expect(serverSentenceOr(message, "操作未完成，請重新確認後再試。")).toBe(expected);
});
