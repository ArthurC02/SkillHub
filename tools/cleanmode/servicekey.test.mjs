import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { createServer } from "node:http";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

import {
  gatewayAdminKey,
  gatewayModels,
  llmChildEnv,
  mintServiceKey,
  serviceKeyAlias,
  serviceKeyPlan,
  verifyServiceKeyBudget,
} from "./servicekey.mjs";

test("launcher starts a supplied key only after a readable budget check", async (t) => {
  for (const maxBudget of [null, 1]) {
    await t.test(
      maxBudget === null ? "unbounded key is refused" : "bounded key starts",
      async (t) => {
        let calls = 0;
        const server = createServer((req, res) => {
          calls++;
          assert.equal(req.url, "/key/info");
          assert.equal(req.headers.authorization, "Bearer sk-fake-service");
          res.setHeader("content-type", "application/json");
          res.end(
            JSON.stringify({ info: { max_budget: maxBudget, spend: 0 } }),
          );
        });
        server.listen(0, "127.0.0.1");
        await once(server, "listening");
        t.after(() => new Promise((resolve) => server.close(resolve)));
        const base = `http://127.0.0.1:${server.address().port}`;
        const child = spawn(
          process.execPath,
          [
            fileURLToPath(new URL("./with-service-key.mjs", import.meta.url)),
            "--",
            process.execPath,
            "--version",
          ],
          {
            timeout: 10000,
            env: {
              ...process.env,
              SKILLHUB_MODEL_GATEWAY_URL: base,
              SKILLHUB_MODEL_GATEWAY_ADMIN_URL: base,
              SKILLHUB_MODEL_GATEWAY_KEY: "sk-fake-admin",
              LITELLM_MASTER_KEY: "sk-fake-admin",
              LITELLM_API_KEY: "sk-fake-service",
            },
          },
        );
        let stdout = "",
          stderr = "";
        child.stdout.on("data", (chunk) => {
          stdout += chunk;
        });
        child.stderr.on("data", (chunk) => {
          stderr += chunk;
        });
        const [code] = await once(child, "close");
        assert.equal(calls, 1);
        assert.equal(code, maxBudget === null ? 1 : 0, stderr);
        if (maxBudget === null) {
          assert.equal(stdout, "");
          assert.match(stderr, /無法確認服務金鑰/);
        } else {
          assert.equal(stdout.trim(), process.version);
        }
        assert.ok(!stderr.includes("sk-fake"));
      },
    );
  }
});

test("service key budget accepts zero spend and the value just below its cap", async () => {
  for (const spend of [0, 1 - Number.EPSILON]) {
    let calls = 0;
    await verifyServiceKeyBudget({
      adminUrl: "http://gateway.test/",
      key: "sk-fake-service",
      fetchImpl: async (url, options) => {
        calls++;
        assert.equal(url, "http://gateway.test/key/info");
        assert.equal(options.headers.Authorization, "Bearer sk-fake-service");
        assert.ok(options.signal instanceof AbortSignal);
        return {
          ok: true,
          json: async () => ({ info: { max_budget: 1, spend } }),
        };
      },
    });
    assert.equal(calls, 1);
  }
});

test("service key budget rejects absent, malformed and exhausted readings", async () => {
  for (const info of [
    undefined,
    null,
    {},
    ...[undefined, null, 0, -1, Infinity, NaN, "1", true].map((max_budget) => ({
      max_budget,
      spend: 0,
    })),
    ...[
      undefined,
      null,
      -Number.EPSILON,
      Infinity,
      NaN,
      "0",
      false,
      1,
      1 + Number.EPSILON,
    ].map((spend) => ({ max_budget: 1, spend })),
  ]) {
    await assert.rejects(
      verifyServiceKeyBudget({
        adminUrl: "http://gateway.test",
        key: "sk-fake-service",
        fetchImpl: async () => ({ ok: true, json: async () => ({ info }) }),
      }),
      /無法確認服務金鑰/,
    );
  }
});

test("service key budget fails closed without exposing transport or response secrets", async () => {
  for (const fetchImpl of [
    async () => {
      throw new Error("sk-secret-transport");
    },
    async () => ({ ok: false, status: 401 }),
    async () => ({
      ok: true,
      json: async () => {
        throw new Error("sk-secret-json");
      },
    }),
  ]) {
    await assert.rejects(
      verifyServiceKeyBudget({
        adminUrl: "http://gateway.test",
        key: "sk-fake-service",
        fetchImpl,
      }),
      (error) => {
        assert.match(error.message, /無法確認服務金鑰/);
        assert.ok(!error.message.includes("sk-"));
        return true;
      },
    );
  }
});

test("mints when nothing is configured yet", () => {
  const plan = serviceKeyPlan({
    SKILLHUB_MODEL_GATEWAY_URL: "http://127.0.0.1:4000",
    SKILLHUB_MODEL_GATEWAY_KEY: "sk-fake-master",
  });
  assert.equal(plan.action, "mint");
});

test("mints over the master key rather than handing it to apps/llm", () => {
  const plan = serviceKeyPlan({
    SKILLHUB_MODEL_GATEWAY_URL: "http://127.0.0.1:4000",
    SKILLHUB_MODEL_GATEWAY_KEY: "sk-fake-master",
    LITELLM_MASTER_KEY: "sk-fake-master",
    LITELLM_API_KEY: "sk-fake-master",
  });
  assert.equal(plan.action, "mint");
});

test("mints with the master key when no dedicated admin key is configured", () => {
  const plan = serviceKeyPlan({
    SKILLHUB_MODEL_GATEWAY_URL: "http://127.0.0.1:4000",
    LITELLM_MASTER_KEY: "sk-fake-master",
  });
  assert.equal(plan.action, "mint");
});

test("uses the master key for the signing request when no dedicated admin key exists", () => {
  assert.equal(
    gatewayAdminKey({ LITELLM_MASTER_KEY: "sk-fake-master" }),
    "sk-fake-master",
  );
});

test("mints over the admin key too", () => {
  const plan = serviceKeyPlan({
    SKILLHUB_MODEL_GATEWAY_URL: "http://127.0.0.1:4000",
    SKILLHUB_MODEL_GATEWAY_KEY: "sk-fake-admin",
    LITELLM_API_KEY: "sk-fake-admin",
  });
  assert.equal(plan.action, "mint");
});

test("SKILLHUB_MODEL_GATEWAY_ADMIN_URL alone is enough to plan a mint", () => {
  const plan = serviceKeyPlan({
    SKILLHUB_MODEL_GATEWAY_ADMIN_URL: "http://127.0.0.1:4001",
    SKILLHUB_MODEL_GATEWAY_KEY: "sk-fake-admin",
  });
  assert.equal(plan.action, "mint");
});

test("keeps an operator-supplied distinct key", () => {
  const plan = serviceKeyPlan({
    SKILLHUB_MODEL_GATEWAY_URL: "http://127.0.0.1:4000",
    SKILLHUB_MODEL_GATEWAY_KEY: "sk-fake-master",
    LITELLM_API_KEY: "sk-fake-service-own",
  });
  assert.deepEqual(plan, {
    action: "keep",
    reason: "an operator supplied a distinct LITELLM_API_KEY",
  });
});

test("skips when no gateway is configured", () => {
  assert.equal(serviceKeyPlan({}).action, "skip");
  assert.equal(
    serviceKeyPlan({ SKILLHUB_MODEL_GATEWAY_URL: "http://127.0.0.1:4000" })
      .action,
    "skip",
  );
});

test("service key aliases keep the stable label while making each mint unique", () => {
  assert.equal(
    serviceKeyAlias(undefined, "first"),
    "skillhub-llm-service-first",
  );
  assert.equal(serviceKeyAlias("interactive", "second"), "interactive-second");
  assert.notEqual(
    serviceKeyAlias(undefined, "first"),
    serviceKeyAlias(undefined, "second"),
  );
});

test("mintServiceKey posts to /key/generate with the admin bearer and returns the key", async () => {
  let seenUrl, seenHeaders, seenBody;
  const fetchImpl = async (url, opts) => {
    seenUrl = url;
    seenHeaders = opts.headers;
    seenBody = JSON.parse(opts.body);
    return {
      ok: true,
      status: 200,
      json: async () => ({ key: "sk-fake-minted" }),
    };
  };
  const key = await mintServiceKey({
    fetchImpl,
    adminUrl: "http://127.0.0.1:4000",
    adminKey: "sk-fake-admin",
    models: ["gpt-5.4-mini"],
    budgetUsd: 1.0,
    alias: "skillhub-llm-service",
  });
  assert.equal(key, "sk-fake-minted");
  assert.equal(seenUrl, "http://127.0.0.1:4000/key/generate");
  assert.equal(seenHeaders.Authorization, "Bearer sk-fake-admin");
  assert.deepEqual(seenBody, {
    key_alias: "skillhub-llm-service",
    duration: "24h",
    max_budget: 1.0,
    models: ["gpt-5.4-mini"],
  });
});

test("mintServiceKey does not double the slash when adminUrl already ends in one", async () => {
  let seenUrl;
  const fetchImpl = async (url) => {
    seenUrl = url;
    return {
      ok: true,
      status: 200,
      json: async () => ({ key: "sk-fake-minted" }),
    };
  };
  await mintServiceKey({
    fetchImpl,
    adminUrl: "http://127.0.0.1:4000/",
    adminKey: "sk-fake-admin",
    models: [],
    budgetUsd: 1.0,
    alias: "skillhub-llm-service",
  });
  assert.equal(seenUrl, "http://127.0.0.1:4000/key/generate");
});

test("mintServiceKey throws (no key value) on a non-2xx response", async () => {
  const fetchImpl = async () => ({
    ok: false,
    status: 401,
    json: async () => ({}),
  });
  await assert.rejects(
    mintServiceKey({
      fetchImpl,
      adminUrl: "http://127.0.0.1:4000",
      adminKey: "sk-fake-admin",
      models: [],
      budgetUsd: 1.0,
      alias: "skillhub-llm-service",
    }),
    /簽發.*失敗/,
  );
});

test("llmChildEnv never carries the master key or the admin key into apps/llm", () => {
  const env = llmChildEnv(
    {
      PATH: "/usr/bin",
      LITELLM_BASE_URL: "http://127.0.0.1:4000",
      LITELLM_MASTER_KEY: "sk-fake-master",
      SKILLHUB_MODEL_GATEWAY_KEY: "sk-fake-admin",
      LITELLM_API_KEY: "sk-fake-master",
    },
    "sk-fake-virtual",
  );
  assert.equal(env.LITELLM_API_KEY, "sk-fake-virtual");
  assert.equal(env.LITELLM_MASTER_KEY, undefined);
  assert.equal(env.SKILLHUB_MODEL_GATEWAY_KEY, undefined);
  assert.equal(env.PATH, "/usr/bin");
  assert.equal(env.LITELLM_BASE_URL, "http://127.0.0.1:4000");
});

test("llmChildEnv with no key leaves apps/llm without LITELLM_API_KEY rather than a stale one", () => {
  const env = llmChildEnv(
    { LITELLM_API_KEY: "sk-fake-master", LITELLM_MASTER_KEY: "sk-fake-master" },
    "",
  );
  assert.equal(env.LITELLM_API_KEY, undefined);
  assert.equal(env.LITELLM_MASTER_KEY, undefined);
});

test("gatewayModels reads model_name entries and nothing else", () => {
  const text =
    "model_list:\n  - model_name: gpt-4.1-mini\n    litellm_params:\n      model: openai/gpt-4.1-mini\n  - model_name: text-embedding-3-small\n";
  assert.deepEqual(gatewayModels(text), [
    "gpt-4.1-mini",
    "text-embedding-3-small",
  ]);
  assert.deepEqual(gatewayModels(""), []);
});

test("mintServiceKey throws when the response has no key field", async () => {
  const fetchImpl = async () => ({
    ok: true,
    status: 200,
    json: async () => ({}),
  });
  await assert.rejects(
    mintServiceKey({
      fetchImpl,
      adminUrl: "http://127.0.0.1:4000",
      adminKey: "sk-fake-admin",
      models: [],
      budgetUsd: 1.0,
      alias: "skillhub-llm-service",
    }),
    /沒有 key 欄位/,
  );
});
