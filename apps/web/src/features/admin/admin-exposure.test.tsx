import { expect, test } from "vitest";
import { ApiError } from "../../core/api/client";
import { WriteFailure } from "./components/WriteFailure";
import {
  ADMIN_EXPOSURE_CASE,
  PUBLICATION,
  PUBLISHER,
  platformResponse,
} from "../../testing/fixtures/platform";
import {
  container,
  calls,
  stub,
  mountAt,
  waitFor,
  has,
  field,
  type,
  button,
  click,
  render,
} from "./testing/harness";

const EXPOSURE_PUBLICATION = `${PUBLISHER}/${PUBLICATION}`;

test("DISC-007: the queue lists a waiting release, and reviewing it shows the exact snapshot search holds", async () => {
  stub(true);
  await mountAt("/admin/exposure");
  await waitFor(has(EXPOSURE_PUBLICATION));
  expect(has("曾核准，之後內容有變，需要重新審核。")()).toBe(true);
  expect(has("待審：共 1 筆。")()).toBe(true);
  expect(has("這份待審清單上次取得於")()).toBe(true);
  expect(
    field<HTMLAnchorElement>(`a[aria-label="審這一筆：${EXPOSURE_PUBLICATION}"]`).textContent,
  ).toBe("審這一筆");

  await click(
    Array.from(container.querySelectorAll("a")).find((a) => a.textContent === "審這一筆")!,
  );
  await waitFor(has("目前未曝光"));
  expect(container.querySelector("h2")?.textContent).toBe(`審這一筆：${EXPOSURE_PUBLICATION}`);
  expect(has("待審清單")()).toBe(true);
  expect(has("曾核准，之後內容有變，需要重新審核。")()).toBe(false);
  expect(new URLSearchParams(window.location.search).get("publication")).toBe(EXPOSURE_PUBLICATION);
  expect(has(ADMIN_EXPOSURE_CASE.snapshot.enriched_summary)()).toBe(true);
  expect(has(ADMIN_EXPOSURE_CASE.snapshot.task_examples)()).toBe(true);
  expect(has("pdf、summary")()).toBe(true);
  expect(has("內容符合規範")()).toBe(true);
  expect(has("需要 python 才能執行，但限制沒有寫出來。")()).toBe(true);
  expect(has("加強摘要用了「professional」這類評價，原文件沒有這樣說。")()).toBe(true);
  expect(has("英文的任務範例裡混了中日文字。")()).toBe(true);
  expect(
    field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]').checked,
  ).toBe(false);
  expect(
    field<HTMLInputElement>('input[name="admin-exposure-decision"][value="revoked"]').checked,
  ).toBe(false);
  expect(button("送出審核結論").disabled).toBe(true);

  await click(
    Array.from(container.querySelectorAll("a")).find((a) => a.textContent === "返回待審清單")!,
  );
  await waitFor(has("曾核准，之後內容有變，需要重新審核。"));
  expect(new URLSearchParams(window.location.search).has("publication")).toBe(false);
});

test("DISC-007: submitting a review sends this screen's release_id, sequence and snapshot digest", async () => {
  stub(true);
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(() => calls.some((c) => c.method === "POST"));
  expect(calls.find((c) => c.method === "POST")).toEqual({
    method: "POST",
    url: `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`,
    body: {
      release_id: ADMIN_EXPOSURE_CASE.release.release_id,
      expected_sequence: ADMIN_EXPOSURE_CASE.sequence,
      expected_snapshot_digest: ADMIN_EXPOSURE_CASE.snapshot.digest,
      decision: "approved",
      reason: "看過了，符合規範",
    },
  });
});

test("a completed exposure decision does not describe the next decision", async () => {
  stub(true, (path, method) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure` && method === "POST"
      ? { body: ADMIN_EXPOSURE_CASE, status: 200 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(has("這筆曝光審核已核准。"));

  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="revoked"]'));
  await waitFor(() => !has("這筆曝光審核已核准。")());
  expect(has("這筆曝光審核已核准。")()).toBe(false);
});

test("a completed exposure review keeps its result when the server advances the review sequence", async () => {
  const updated = {
    ...ADMIN_EXPOSURE_CASE,
    sequence: 3,
    exposed: true,
    history: [
      { ...ADMIN_EXPOSURE_CASE.history[0], sequence: 3, reason: "看過了，符合規範" },
      ...ADMIN_EXPOSURE_CASE.history,
    ],
  };
  let reviewed = false;
  stub(true, (path, method) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    if (method === "POST") {
      reviewed = true;
      return { body: updated, status: 200 };
    }
    return { body: reviewed ? updated : ADMIN_EXPOSURE_CASE, status: 200 };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(has("審核序號：3"));
  expect(has("目前曝光中")()).toBe(true);
  expect(has("這筆曝光審核已核准。")()).toBe(true);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-exposure-result"));
});

test("editing a new exposure reason clears the previous review result", async () => {
  stub(true, (path, method) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure` && method === "POST"
      ? { body: ADMIN_EXPOSURE_CASE, status: 200 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(has("這筆曝光審核已核准。"));
  await type("#admin-exposure-review-note", "需要重新確認");
  await waitFor(() => !has("這筆曝光審核已核准。")());
  expect(has("這筆曝光審核已核准。")()).toBe(false);
});

test("a completed exposure review stays confirmed when rereading the current state fails", async () => {
  const updated = { ...ADMIN_EXPOSURE_CASE, sequence: 3, exposed: true };
  let reviewed = false;
  stub(true, (path, method) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    if (method === "POST") {
      reviewed = true;
      return { body: updated, status: 200 };
    }
    return reviewed
      ? { body: { error: "refresh unavailable" }, status: 503 }
      : { body: ADMIN_EXPOSURE_CASE, status: 200 };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(has("暫時無法讀取這一筆的曝光審核資料"));
  expect(has("這筆曝光審核已核准。")()).toBe(true);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-exposure-result"));
});

test("DISC-007: a decision and reason are both required before submission", async () => {
  stub(true);
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  expect(button("送出審核結論").disabled).toBe(true);
  await type("#admin-exposure-review-note", "看過了");
  expect(button("送出審核結論").disabled).toBe(true);
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  expect(button("送出核准").disabled).toBe(false);
});

test("DISC-007: a decision chosen without a reason keeps submission disabled", async () => {
  stub(true);
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  expect(button("送出核准").disabled).toBe(true);
});

test("DISC-007: an empty queue is named as a genuine zero, not a blank list", async () => {
  stub(true, (path) =>
    path === "/admin/exposure-reviews" ? { body: { publications: [] }, status: 200 } : undefined,
  );
  await mountAt("/admin/exposure");
  await waitFor(has("沒有等待審核的發佈物：0 筆。"));
  expect(container.querySelectorAll(".download-item")).toHaveLength(0);
});

test("DISC-007: a failed queue refresh hides stale work and retry restores the latest queue", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/exposure-reviews") return undefined;
    reads += 1;
    if (reads === 2) return { body: { error: "service unavailable" }, status: 503 };
    return reads === 1 ? platformResponse(path) : { body: { publications: [] }, status: 200 };
  });
  await mountAt("/admin/exposure");
  await waitFor(has("待審：共 1 筆。"));

  await click(button("重新整理"));
  await waitFor(has("暫時無法讀取待審清單"));
  expect(container.querySelectorAll(".download-item")).toHaveLength(0);
  expect(has("先前載入的內容已隱藏。")()).toBe(true);

  await click(button("再試一次"));
  await waitFor(has("沒有等待審核的發佈物：0 筆。"));
  expect(reads).toBe(3);
});

test("DISC-007: an initial queue failure offers retry without claiming the queue is empty", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/exposure-reviews") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: { publications: [] }, status: 200 };
  });
  await mountAt("/admin/exposure");
  await waitFor(has("暫時無法讀取待審清單"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(has("沒有等待審核的發佈物：0 筆。")()).toBe(false);

  await click(button("再試一次"));
  await waitFor(has("沒有等待審核的發佈物：0 筆。"));
  expect(reads).toBe(2);
});

test("DISC-007: a stale review retains its actionable Chinese reason", async () => {
  const staleMessage =
    "這份審核的前提已經過期：有新的 Release，或別人已經審過。重新打開這一筆，看過現在的內容再送出";
  stub(true, (path, method) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure` && method === "POST"
      ? { body: { error: staleMessage, reason: "review_stale" }, status: 409 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了");
  await click(button("送出核准"));
  await waitFor(has(`操作未被接受：${staleMessage}`));
});

test("a stale exposure review reloads the current case before another decision", async () => {
  let stale = false;
  stub(true, (path, method) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    if (method === "POST") {
      stale = true;
      return { body: { error: "審核前提已過期", reason: "review_stale" }, status: 409 };
    }
    return {
      body: {
        ...ADMIN_EXPOSURE_CASE,
        sequence: stale ? 3 : 2,
        exposed: stale,
      },
      status: 200,
    };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過舊版");
  await click(button("送出核准"));

  await waitFor(has("審核序號：3"));
  expect(
    calls.filter((call) => call.method === "GET" && call.url.endsWith("/exposure")).length,
  ).toBeGreaterThanOrEqual(2);
  expect(
    field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]').checked,
  ).toBe(false);
  expect(field<HTMLTextAreaElement>("#admin-exposure-review-note").value).toBe("");
  expect(has("案件已更新，請重新檢查內容後再決定。")()).toBe(true);
});

test.each([
  [403, "操作未被接受；目前沒有執行權限，請確認登入身分。"],
  [429, "操作未被接受；送出太頻繁，請稍後再試。"],
])("an admin write refused with %i gives an actionable next step", async (status, expected) => {
  await render(<WriteFailure error={new ApiError(status, "server detail")} />);
  expect(container.textContent).toContain(expected);
  expect(container.textContent).not.toContain("server detail");
});

test("an English server diagnostic with a Chinese fragment stays hidden", async () => {
  await render(<WriteFailure error={new ApiError(409, "server detail：中文")} />);
  expect(container.textContent).toContain("操作未被接受；目前狀態可能已變更，請重新整理後再判斷。");
  expect(container.textContent).not.toContain("server detail");
});

test.each([
  ["never checked", null, "這一版補充時還沒有自檢，沒有結果可看。"],
  ["checked and clean", [], "沒有發現與原文件不一致的地方。"],
])("DISC-007: an enrichment %s says so in its own words", async (_name, findings, sentence) => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? {
          body: {
            ...ADMIN_EXPOSURE_CASE,
            snapshot: { ...ADMIN_EXPOSURE_CASE.snapshot, enrichment_findings: findings },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has(sentence));
  expect(has("需要 python 才能執行")()).toBe(false);
});

test("DISC-007: a release search has not indexed yet says so instead of showing stale text", async () => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? {
          body: {
            ...ADMIN_EXPOSURE_CASE,
            snapshot: undefined,
            approval: {
              allowed: false,
              refusal: {
                reason: "snapshot_not_current",
                error: "搜尋索引裡的內容不是這一筆 Release 的版本",
              },
            },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("尚未進索引"));
  expect(has(ADMIN_EXPOSURE_CASE.snapshot.enriched_summary)()).toBe(false);
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
  expect(has("搜尋索引裡的內容不是這一筆 Release 的版本")()).toBe(true);
  expect(field<HTMLInputElement>('input[value="revoked"]').disabled).toBe(false);
});

test.each([
  [
    "搜尋索引仍指向另一版",
    { ...ADMIN_EXPOSURE_CASE.snapshot, current: false },
    "snapshot_not_current",
    "搜尋索引裡的內容不是這一筆 Release 的版本",
  ],
  [
    "搜尋內容還在補充",
    { ...ADMIN_EXPOSURE_CASE.snapshot, enriched: false },
    "snapshot_pending",
    "這一版的搜尋內容還在補充",
  ],
])("DISC-007: %s時不能核准曝光", async (_condition, snapshot, reason, error) => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? {
          body: {
            ...ADMIN_EXPOSURE_CASE,
            snapshot,
            approval: { allowed: false, refusal: { reason, error } },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
  expect(has(error)()).toBe(true);
  expect(field<HTMLInputElement>('input[value="revoked"]').disabled).toBe(false);
});

test("DISC-007: a withdrawn publication cannot be approved again", async () => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? {
          body: {
            ...ADMIN_EXPOSURE_CASE,
            status: "delisted",
            approval: {
              allowed: false,
              refusal: { reason: "not_published", error: "這個發佈物已經撤回，不能核准曝光" },
            },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("這個發佈物已經撤回，不能核准曝光"));
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
  expect(field<HTMLInputElement>('input[value="revoked"]').disabled).toBe(false);
});

test.each([
  [
    "redistribution_not_allowed",
    "可散布判定不是 allowed：要先以既有的可散布判定動詞附授權證據判成 allowed，才能核准曝光",
  ],
  ["not_available", "這個 Skill 目前被下架、被保留或已刪除，不能核准曝光"],
])(
  "DISC-007: %s blocks approval before submission and preserves revocation",
  async (reason, error) => {
    stub(true, (path) =>
      path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
        ? {
            body: {
              ...ADMIN_EXPOSURE_CASE,
              approval: { allowed: false, refusal: { reason, error } },
            },
            status: 200,
          }
        : undefined,
    );
    await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
    await waitFor(has("審核這一版"));
    expect(has(error)()).toBe(true);
    const approved = field<HTMLInputElement>('input[value="approved"]');
    expect(approved.disabled).toBe(true);
    expect(approved.getAttribute("aria-describedby")).toBe("admin-exposure-approval-why");
    expect(field<HTMLInputElement>('input[value="revoked"]').disabled).toBe(false);
  },
);

test("DISC-007: missing approval evidence does not make approval available", async () => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? { body: { ...ADMIN_EXPOSURE_CASE, approval: undefined }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("無法確認核准資格；請重新整理審核資料。"));
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
});

test("DISC-007: changed approval evidence clears a draft based on the old eligibility", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    reads += 1;
    return {
      body:
        reads === 1
          ? ADMIN_EXPOSURE_CASE
          : {
              ...ADMIN_EXPOSURE_CASE,
              approval: {
                allowed: false,
                refusal: { reason: "not_available", error: "這個 Skill 目前被保留，不能核准曝光" },
              },
            },
      status: 200,
    };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[value="approved"]'));
  await type("#admin-exposure-review-note", "原先看到的資料");

  await click(button("重新整理審核資料"));
  await waitFor(has("這個 Skill 目前被保留，不能核准曝光"));
  expect(field<HTMLInputElement>('input[value="approved"]').checked).toBe(false);
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
  expect(field<HTMLTextAreaElement>("#admin-exposure-review-note").value).toBe("");
});

test("DISC-007: a failed refresh hides a previously loaded case and its approval control", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    reads += 1;
    return reads === 1
      ? { body: ADMIN_EXPOSURE_CASE, status: 200 }
      : { body: { error: "service unavailable" }, status: 503 };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(button("重新整理審核資料"));
  await waitFor(has("暫時無法讀取這一筆的曝光審核資料"));
  expect(reads).toBe(2);
  expect(container.querySelector('input[value="approved"]')).toBeNull();
});

test("DISC-007: a new review premise clears the previous decision and reason", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    reads += 1;
    return {
      body: {
        ...ADMIN_EXPOSURE_CASE,
        sequence: reads === 1 ? 2 : 3,
        release: {
          ...ADMIN_EXPOSURE_CASE.release,
          version_number: reads === 1 ? 1 : 2,
        },
      },
      status: 200,
    };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(field<HTMLInputElement>('input[value="approved"]'));
  await type("#admin-exposure-review-note", "已看過舊版");
  expect(button("送出核准").disabled).toBe(false);

  await click(button("重新整理審核資料"));
  await waitFor(has("審核序號：3"));
  expect(field<HTMLInputElement>('input[value="approved"]').checked).toBe(false);
  expect(field<HTMLTextAreaElement>("#admin-exposure-review-note").value).toBe("");
  expect(button("送出審核結論").disabled).toBe(true);
});
