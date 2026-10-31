import { expect, test } from "vitest";
import { ApiError } from "../../core/api/client";
import { acquisitionFailureSentence, writeFailureSentence } from "./publishing.model";

const DOWNLOAD_REFUSAL = "這個 Skill 的授權不允許再散布，所以無法從中產出套件";

test.each([
  {
    name: "a 422 download refusal worded by the server",
    error: new ApiError(422, DOWNLOAD_REFUSAL, { reason: "not_redistributable" }),
    want: DOWNLOAD_REFUSAL,
  },
  {
    name: "a 422 refusal in English",
    error: new ApiError(422, "not redistributable", { reason: "not_redistributable" }),
    want: "這個小工具的授權不允許再散布，所以不能發佈。",
  },
  {
    name: "a 409 refusal the table names",
    error: new ApiError(409, "這次操作無法完成。", { reason: "name_taken" }),
    want: "這個名稱已經被別人取走了，換一個名稱再試一次。",
  },
  {
    name: "a 400 without a reason",
    error: new ApiError(400, "bad request"),
    want: "下載沒有成功，可以再試一次。",
  },
])("acquisitionFailureSentence: $name", ({ error, want }) => {
  expect(acquisitionFailureSentence(error, "下載沒有成功，可以再試一次。")).toBe(want);
});

test.each([
  {
    status: 404,
    want: "發佈沒有成功：要用到的小工具、版本或發佈物已經不在了，重送也不會成功；重新整理頁面看看最新狀態。",
  },
  { status: 400, want: "發佈沒有成功，可以再試一次。" },
  { status: 403, want: "發佈沒有成功，可以再試一次。" },
])("writeFailureSentence says whether resending can help after a $status", ({ status, want }) => {
  expect(writeFailureSentence(new ApiError(status, "not found"), "發佈")).toBe(want);
});
