import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { ApiError } from "../../core/api/client";
import { MutationError } from "./test-cases/components/MutationError";

test("an unreadable write result remains unconfirmed rather than reporting a failed write", () => {
  const markup = renderToStaticMarkup(
    <MutationError
      error={new ApiError(500, "projection-private-canary")}
      what="測試題"
      fallback="儲存沒有成功，可以再試一次。"
    />,
  );
  expect(markup).toContain("操作結果尚未確認；請先確認最新資料，再決定是否重送。");
  expect(markup).not.toContain("儲存沒有成功");
  expect(markup).not.toContain("projection-private-canary");
});

test("an operation-specific uncertain result keeps its own recovery wording", () => {
  const markup = renderToStaticMarkup(
    <MutationError
      error={new ApiError(500, "projection-private-canary")}
      what="測試題"
      fallback="儲存沒有成功，可以再試一次。"
      unknown="這次送出結果尚未確認。"
    />,
  );
  expect(markup).toContain("這次送出結果尚未確認。");
  expect(markup).not.toContain("操作結果尚未確認");
});

test("an explicit refusal retains its known reason", () => {
  const markup = renderToStaticMarkup(
    <MutationError
      error={new ApiError(400, "invalid", { error: "名稱不能空白" })}
      what="測試題"
      fallback="儲存沒有成功，可以再試一次。"
    />,
  );
  expect(markup).toContain("名稱不能空白");
  expect(markup).not.toContain("結果尚未確認");
});
