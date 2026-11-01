import { act } from "react";
import { expect, test } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { ADMIN_ACCOUNT, ADMIN_LEDGER } from "../../testing/fixtures/platform";
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
  submit,
} from "./testing/harness";

async function lookUp(email: string) {
  await mountAt("/admin/accounts");
  await waitFor(has("誰在何時查了誰"));
  await type("#admin-account-email", email);
  await submit("#admin-account-email");
}

test("OPS-002: a lookup keeps the email out of the address and shows the account with its ledger", async () => {
  stub(true);
  await lookUp("Member@Example.com");
  await waitFor(has("封測者甲"));
  await waitFor(has("目前餘額"));
  expect(calls.some((c) => c.url === "/admin/accounts?email=Member%40Example.com")).toBe(true);
  expect(window.location.href).not.toContain("xample");
  const rows = Array.from(container.querySelectorAll("tbody tr")).map((tr) =>
    Array.from(tr.querySelectorAll("td"))
      .slice(1)
      .map((td) => td.textContent),
  );
  expect(rows).toEqual([
    ["扣點（估計值）", "-30", "run"],
    ["授予", "+150", "不適用"],
  ]);
  expect(field<HTMLElement>(".table-scroll").tabIndex).toBe(0);
  expect(field("strong").textContent).toBe("120");
  expect(has("封測准入目前可通過；可能是已受邀，或此部署未限制。")()).toBe(true);
});

test("OPS-002: submitting the same email again performs a fresh audited lookup", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/accounts") return undefined;
    reads += 1;
    return {
      body: { ...ADMIN_ACCOUNT, display_name: `第 ${reads} 次查詢` },
      status: 200,
    };
  });
  await lookUp("member@example.com");
  await waitFor(has("第 1 次查詢"));
  expect(reads).toBe(1);

  await submit("#admin-account-email");
  await waitFor(has("第 2 次查詢"));
  expect(reads).toBe(2);
});

test("an account denied by beta admission is not described as merely absent from a roster", async () => {
  stub(true, (path) =>
    path === "/admin/accounts"
      ? { body: { ...ADMIN_ACCOUNT, in_beta_allowlist: false }, status: 200 }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("目前不可通過；請確認封測名單或部署設定。"));
  expect(has("不在名單上")()).toBe(false);
});

test("OPS-002: an email nobody has is named as such, not reported as a broken read", async () => {
  stub(true, (path) =>
    path === "/admin/accounts" ? { body: { error: "not found" }, status: 404 } : undefined,
  );
  await lookUp("ghost@example.com");
  await waitFor(has("沒有 email 是「ghost@example.com」的帳號"));
  expect(container.querySelector('[role="alert"]')).toBeNull();
});

test("OPS-002: editing the email hides the prior account until the new lookup completes", async () => {
  stub(true, (path, _method, url) =>
    path === "/admin/credits/f3333333-3333-4333-8333-333333333333"
      ? {
          body: { ...ADMIN_LEDGER, workspace_id: "f3333333-3333-4333-8333-333333333333" },
          status: 200,
        }
      : path === "/admin/accounts" && url.includes("other%40example.com")
        ? {
            body: {
              ...ADMIN_ACCOUNT,
              email: "other@example.com",
              display_name: "封測者乙",
              workspace_id: "f3333333-3333-4333-8333-333333333333",
            },
            status: 200,
          }
        : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("封測者甲"));
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "50");
  await type("#admin-grant-note", "first account");

  await type("#admin-account-email", "MEMBER@example.com");
  expect(has("封測者甲")()).toBe(true);
  await type("#admin-account-email", "other@example.com");
  expect(has("Email 已變更；按「查詢」載入新帳號。")()).toBe(true);
  expect(has("封測者甲")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();
  expect(calls.some((call) => call.url.includes("other%40example.com"))).toBe(false);

  await submit("#admin-account-email");
  await waitFor(has("封測者乙"));
  await waitFor(has("授予點數"));
  expect(field<HTMLInputElement>("#admin-grant-amount").value).toBe("");
  expect(field<HTMLTextAreaElement>("#admin-grant-note").value).toBe("");
  expect(button("授予").disabled).toBe(true);
});

test("OPS-002: a failed account reread hides the old target and offers retry", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/accounts") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_ACCOUNT, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.account("member@example.com") });
  });
  await waitFor(has("暫時無法讀取帳號"));
  expect(has("封測者甲")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();

  await click(button("再試一次"));
  await waitFor(has("封測者甲"));
  expect(reads).toBe(3);
});

test("OPS-002: a first lookup failure offers retry without inventing an account", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/accounts") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_ACCOUNT, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("暫時無法讀取帳號"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(has("封測者甲")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();

  await click(button("再試一次"));
  await waitFor(has("封測者甲"));
  expect(reads).toBe(2);
});

test("OPS-003: a failed ledger reread hides stale balance and blocks a grant until retry", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/credits/f2222222-2222-4222-8222-222222222222") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_LEDGER, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("目前餘額"));
  await type("#admin-grant-amount", "50");
  await type("#admin-grant-note", "correction");
  expect(button("授予").disabled).toBe(false);

  await act(async () => {
    await queryClient.refetchQueries({
      queryKey: queryKeys.admin.ledger("f2222222-2222-4222-8222-222222222222"),
    });
  });
  await waitFor(has("暫時無法讀取點數"));
  expect(has("目前餘額")()).toBe(false);
  expect(button("授予").disabled).toBe(true);
  expect(has("要等最新餘額讀取完成，才可授予點數。")()).toBe(true);
  expect(has("「授予」要等上面的欄位都填好。")()).toBe(false);
  expect(button("授予").getAttribute("aria-describedby")).toBe("admin-grant-why");
  expect(field<HTMLElement>("#admin-grant-why").textContent).toBe(
    "要等最新餘額讀取完成，才可授予點數。",
  );

  await click(button("再試一次"));
  await waitFor(has("目前餘額"));
  expect(button("授予").disabled).toBe(false);
  expect(reads).toBe(3);
});

test("OPS-003: a first ledger read failure offers retry without a grant form", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/credits/f2222222-2222-4222-8222-222222222222") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_LEDGER, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("暫時無法讀取點數"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(has("目前餘額")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();

  await click(button("再試一次"));
  await waitFor(has("目前餘額"));
  expect(reads).toBe(2);
});

test("OPS-003: a grant waits for a non-zero whole amount and a reason, then posts both and reloads the ledger", async () => {
  stub(true, (path, method) =>
    method === "POST" && path === "/admin/credits/f2222222-2222-4222-8222-222222222222/grants"
      ? {
          body: {
            workspace_id: "f2222222-2222-4222-8222-222222222222",
            balance_credits: 170,
            amount_credits: 50,
          },
          status: 200,
        }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  const grant = () => button("授予");
  expect(grant().disabled).toBe(true);

  await type("#admin-grant-note", "  beta reward ");
  for (const amount of ["0", "1.5", ""]) {
    await type("#admin-grant-amount", amount);
    expect(grant().disabled, `amount 「${amount}」`).toBe(true);
  }
  await type("#admin-grant-amount", "50");
  expect(grant().disabled).toBe(false);

  const ledgerReads = () =>
    calls.filter((c) => c.url === "/admin/credits/f2222222-2222-4222-8222-222222222222").length;
  const before = ledgerReads();
  await click(grant());
  await waitFor(has("已授予 50 點，餘額現在是 170 點。"));
  expect(calls.find((c) => c.method === "POST")?.body).toEqual({
    amount_credits: 50,
    reason: "beta reward",
    idempotency_key: expect.stringMatching(/^[0-9a-f-]{36}$/),
  });
  await waitFor(() => ledgerReads() > before);
  await type("#admin-grant-amount", "75");
  expect(has("已授予 50 點，餘額現在是 170 點。")()).toBe(false);
});

test("OPS-003: a negative correction says it deducts credits before and after submission", async () => {
  stub(true, (path, method) =>
    method === "POST" && path === "/admin/credits/f2222222-2222-4222-8222-222222222222/grants"
      ? {
          body: {
            workspace_id: "f2222222-2222-4222-8222-222222222222",
            balance_credits: 90,
            amount_credits: -30,
          },
          status: 200,
        }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "-30");
  await type("#admin-grant-note", "corrects an over-grant");

  expect(has("更正點數")()).toBe(true);
  expect(button("扣減點數").disabled).toBe(false);
  await click(button("扣減點數"));
  await waitFor(has("已扣減 30 點，餘額現在是 90 點。"));
  expect(calls.find((call) => call.method === "POST")?.body).toMatchObject({
    amount_credits: -30,
    reason: "corrects an over-grant",
  });
});

test("OPS-003: a failed grant retries with one key, but a completed draft cannot be sent again", async () => {
  let attempt = 0;
  stub(true, (path, method) => {
    if (method !== "POST" || !path.endsWith("/grants")) return undefined;
    attempt += 1;
    return attempt === 1
      ? { body: { error: "grant failed" }, status: 500 }
      : {
          body: {
            workspace_id: "f2222222-2222-4222-8222-222222222222",
            balance_credits: 60,
            amount_credits: 10,
          },
          status: 200,
        };
  });
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "10");
  await type("#admin-grant-note", "r");
  await click(button("授予"));
  await waitFor(has("無法確認操作是否完成；請先重新整理目前狀態，再決定是否重試。"));
  expect(has("grant failed")()).toBe(false);
  expect(field<HTMLInputElement>("#admin-grant-amount").readOnly).toBe(true);
  expect(field<HTMLTextAreaElement>("#admin-grant-note").readOnly).toBe(true);
  expect(has("金額與理由暫時不能更改")()).toBe(true);
  await click(button("授予"));
  await waitFor(has("已授予 10 點，餘額現在是 60 點。"));
  expect(button("授予").disabled).toBe(true);
  expect(button("授予").getAttribute("aria-describedby")).toBe("admin-grant-why");
  expect(field<HTMLElement>("#admin-grant-why").textContent).toBe(
    "已完成這筆操作；修改上方欄位或理由後再送出，會建立另一筆操作。",
  );
  expect(calls.filter((c) => c.method === "POST")).toHaveLength(2);
  await type("#admin-grant-note", "another grant");
  expect(button("授予").disabled).toBe(false);
  await click(button("授予"));
  await waitFor(() => calls.filter((c) => c.method === "POST").length === 3);
  const keys = calls
    .filter((c) => c.method === "POST")
    .map((c) => (c.body as { idempotency_key: string }).idempotency_key);
  expect(keys[1]).toBe(keys[0]);
  expect(keys[2]).not.toBe(keys[0]);
  expect(field<HTMLInputElement>("#admin-grant-amount").readOnly).toBe(false);
  expect(field<HTMLTextAreaElement>("#admin-grant-note").readOnly).toBe(false);
});

test("OPS-003: a refused grant explains the next step without raw server text", async () => {
  stub(true, (path, method) =>
    method === "POST" && path.endsWith("/grants")
      ? { body: { error: "amount_credits must not be zero" }, status: 400 }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "5");
  await type("#admin-grant-note", "r");
  await click(button("授予"));
  await waitFor(has("操作未被接受；請核對輸入與目前狀態後再試。"));
  expect(has("amount_credits must not be zero")()).toBe(false);
  expect(field<HTMLInputElement>("#admin-grant-amount").readOnly).toBe(false);
  expect(field<HTMLTextAreaElement>("#admin-grant-note").readOnly).toBe(false);
  await type("#admin-grant-note", "修改後的理由");
  expect(has("操作未被接受；請核對輸入與目前狀態後再試。")()).toBe(false);
});

test.each([
  [400, "餘額不能低於 -1000000 點；這次沒有變更任何點數。"],
  [409, "這個帳號正在刪除中；這次沒有授予任何點數。"],
])("OPS-003: a grant refused with %i shows the platform's own reason", async (status, reason) => {
  stub(true, (path, method) =>
    method === "POST" && path.endsWith("/grants") ? { body: { error: reason }, status } : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "5");
  await type("#admin-grant-note", "r");
  await click(button("授予"));
  await waitFor(has(`操作未被接受：${reason}`));
});

test("OPS-003: a grant whose outcome is unknown brings its explanation into view", async () => {
  const scrolled: Element[] = [];
  const original = HTMLElement.prototype.scrollIntoView;
  HTMLElement.prototype.scrollIntoView = function (this: Element) {
    scrolled.push(this);
  };
  try {
    stub(true, (path, method) =>
      method === "POST" && path.endsWith("/grants")
        ? { body: { error: "grant failed" }, status: 500 }
        : undefined,
    );
    await lookUp("member@example.com");
    await waitFor(has("授予點數"));
    await type("#admin-grant-amount", "5");
    await type("#admin-grant-note", "r");
    await click(button("授予"));
    await waitFor(has("無法確認操作是否完成"));

    expect(scrolled.map((element) => element.textContent)).toContain(
      "無法確認操作是否完成；請先重新整理目前狀態，再決定是否重試。",
    );
  } finally {
    if (original) HTMLElement.prototype.scrollIntoView = original;
    else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
  }
});
