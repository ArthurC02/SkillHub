import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ApiError } from "../../../../core/api/client";
import { ApplyFailureBody } from "./ApplyFailureBody";

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test.each([
  {
    error: new ApiError(400, "建議 3 還沒有被採用"),
    says: "建議 3 還沒有被採用|再按一次也不會成功，請重新確認各項建議的套用狀態。",
  },
  {
    error: new ApiError(404, "suggestion not found"),
    says: "套用失敗。|再按一次也不會成功，請重新確認各項建議的套用狀態。",
  },
  { error: new ApiError(409, "建議 3 還沒有被採用"), says: "套用失敗，可以再按一次。" },
])("a $error.status refusal says $says", async ({ error, says }) => {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  await act(async () => root.render(<ApplyFailureBody error={error} />));

  expect(Array.from(container.querySelectorAll("p"), (p) => p.textContent).join("|")).toBe(says);
  await act(async () => root.unmount());
  container.remove();
});
