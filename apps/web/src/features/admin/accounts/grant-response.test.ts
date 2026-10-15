import { afterEach, expect, test, vi } from "vitest";
import { ADMIN_LEDGER, ME } from "../../../testing/fixtures/platform";
import { grantCredits } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

const RECEIPT = {
  workspace_id: ADMIN_LEDGER.workspace_id,
  balance_credits: 145,
  amount_credits: 25,
};

test.each([9007199254740992, -9007199254740992])(
  "an unsafe echoed amount %s cannot confirm a grant",
  async (amount) => {
    vi.stubGlobal("fetch", () =>
      Promise.resolve(
        new Response(JSON.stringify({ ...RECEIPT, amount_credits: amount }), { status: 200 }),
      ),
    );
    await expect(
      grantCredits(RECEIPT.workspace_id, {
        amount_credits: amount,
        reason: "checked grant",
        idempotency_key: "same-grant",
      }),
    ).rejects.toThrow("Invalid credit grant response");
  },
);

test.each<[string, unknown, number]>([
  ["standard", RECEIPT, 25],
  ["correction", { ...RECEIPT, amount_credits: -25 }, -25],
  ["maximum safe amount", { ...RECEIPT, amount_credits: 9007199254740991 }, 9007199254740991],
  ["minimum safe amount", { ...RECEIPT, amount_credits: -9007199254740991 }, -9007199254740991],
  ["zero balance", { ...RECEIPT, balance_credits: 0 }, 25],
  ["negative balance", { ...RECEIPT, balance_credits: -1 }, 25],
  ["maximum safe balance", { ...RECEIPT, balance_credits: 9007199254740991 }, 25],
  ["minimum safe balance", { ...RECEIPT, balance_credits: -9007199254740991 }, 25],
  [
    "equivalent UUID spelling",
    { ...RECEIPT, workspace_id: RECEIPT.workspace_id.toUpperCase() },
    25,
  ],
  [
    "current balance on replay and extra metadata",
    { ...RECEIPT, balance_credits: 300, extra: true },
    25,
  ],
])("a complete %s grant receipt preserves the response", async (_name, response, amount) => {
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(response), { status: 200 })),
  );
  vi.stubGlobal("fetch", fetch);
  const body = {
    amount_credits: amount,
    reason: "checked grant",
    idempotency_key: "same-grant",
  };
  await expect(grantCredits(RECEIPT.workspace_id, body)).resolves.toEqual(response);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe(`/admin/credits/${RECEIPT.workspace_id}/grants`);
  expect(fetch.mock.calls[0]?.[1]?.method).toBe("POST");
  expect(JSON.parse(fetch.mock.calls[0]?.[1]?.body as string)).toEqual(body);
});

test.each<[string, unknown]>([
  ["null body", null],
  ["array body", []],
  ["primitive body", true],
  ["empty body", {}],
  ["missing workspace", { ...RECEIPT, workspace_id: undefined }],
  ["non-string workspace", { ...RECEIPT, workspace_id: {} }],
  ["foreign workspace", { ...RECEIPT, workspace_id: ME.workspace_id }],
  ["missing balance", { ...RECEIPT, balance_credits: undefined }],
  ["null balance", { ...RECEIPT, balance_credits: null }],
  ["string balance", { ...RECEIPT, balance_credits: "145" }],
  ["fractional balance", { ...RECEIPT, balance_credits: 1.5 }],
  ["too large balance", { ...RECEIPT, balance_credits: 9007199254740992 }],
  ["too small balance", { ...RECEIPT, balance_credits: -9007199254740992 }],
  ["missing amount", { ...RECEIPT, amount_credits: undefined }],
  ["null amount", { ...RECEIPT, amount_credits: null }],
  ["string amount", { ...RECEIPT, amount_credits: "25" }],
  ["fractional amount", { ...RECEIPT, amount_credits: 25.5 }],
  ["different amount", { ...RECEIPT, amount_credits: 125 }],
  ["opposite amount", { ...RECEIPT, amount_credits: -25 }],
  ["zero amount", { ...RECEIPT, amount_credits: 0 }],
])("a grant receipt with %s cannot confirm the submitted operation", async (_name, response) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(response), { status: 200 })),
  );
  await expect(
    grantCredits(RECEIPT.workspace_id, {
      amount_credits: 25,
      reason: "checked grant",
      idempotency_key: "same-grant",
    }),
  ).rejects.toThrow("Invalid credit grant response");
});
