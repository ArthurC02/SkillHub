// Tests for how clean mode mints apps/llm's LiteLLM Virtual Key.
//
// `node --test`, no framework, same shape as env.test.mjs.
import assert from "node:assert/strict";
import { test } from "node:test";

import { gatewayModels, llmChildEnv, mintServiceKey, serviceKeyPlan } from "./servicekey.mjs";

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

test("mints over the admin key too", () => {
  const plan = serviceKeyPlan({
    SKILLHUB_MODEL_GATEWAY_URL: "http://127.0.0.1:4000",
    SKILLHUB_MODEL_GATEWAY_KEY: "sk-fake-admin",
    LITELLM_API_KEY: "sk-fake-admin",
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
    serviceKeyPlan({ SKILLHUB_MODEL_GATEWAY_URL: "http://127.0.0.1:4000" }).action,
    "skip",
  );
});

test("mintServiceKey posts to /key/generate with the admin bearer and returns the key", async () => {
  let seenUrl, seenHeaders, seenBody;
  const fetchImpl = async (url, opts) => {
    seenUrl = url;
    seenHeaders = opts.headers;
    seenBody = JSON.parse(opts.body);
    return { ok: true, status: 200, json: async () => ({ key: "sk-fake-minted" }) };
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

test("mintServiceKey throws (no key value) on a non-2xx response", async () => {
  const fetchImpl = async () => ({ ok: false, status: 401, json: async () => ({}) });
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
  const env = llmChildEnv({ LITELLM_API_KEY: "sk-fake-master", LITELLM_MASTER_KEY: "sk-fake-master" }, "");
  assert.equal(env.LITELLM_API_KEY, undefined);
  assert.equal(env.LITELLM_MASTER_KEY, undefined);
});

test("gatewayModels reads model_name entries and nothing else", () => {
  const text = "model_list:\n  - model_name: gpt-4.1-mini\n    litellm_params:\n      model: openai/gpt-4.1-mini\n  - model_name: text-embedding-3-small\n";
  assert.deepEqual(gatewayModels(text), ["gpt-4.1-mini", "text-embedding-3-small"]);
  assert.deepEqual(gatewayModels(""), []);
});

test("mintServiceKey throws when the response has no key field", async () => {
  const fetchImpl = async () => ({ ok: true, status: 200, json: async () => ({}) });
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
