import { StrictMode, act, type ComponentType } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { TEST_CASE_DRAFT } from "../../testing/fixtures/platform";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import { PromptForm } from "./test-cases/components/PromptForm";
import { SimulatedUserForm } from "./test-cases/components/SimulatedUserForm";
import { RubricSection } from "./test-cases/components/RubricSection";
import type { TestCase } from "./testcases.service";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  queryClient.clear();
  vi.unstubAllGlobals();
});

const forms = [
  { name: "prompt", Component: PromptForm, selectors: ["#edit-name", "#edit-prompt"] },
  { name: "facts", Component: SimulatedUserForm, selectors: ["#simulated-user-facts"] },
  { name: "rubric", Component: RubricSection, selectors: ["#rubric-version", "#rubric-c1"] },
] as const;

function freshCase(): TestCase {
  const next = initialCase();
  next.name = "伺服器新名稱";
  next.user_prompt = "伺服器新版任務";
  next.simulated_user_facts = ["伺服器新背景"];
  next.rubric!.version = "server/v2";
  next.rubric!.items[0].text = "伺服器新評分說明";
  return next;
}

function initialCase(): TestCase {
  return structuredClone(TEST_CASE_DRAFT) as TestCase;
}

function field(selector: string) {
  const input = container.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector);
  if (!input) throw new Error(`Missing field ${selector}`);
  return input;
}

async function edit(selector: string, value: string) {
  const input = field(selector);
  const prototype =
    input instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  await act(async () => {
    Object.getOwnPropertyDescriptor(prototype, "value")!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function render(Component: ComponentType<{ testCase: TestCase }>, testCase: TestCase) {
  await act(async () => {
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <Component testCase={testCase} />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
}

for (const form of forms) {
  test.each(["pristine", "dirty", "cleared", "reverted"] as const)(
    `${form.name} keeps field ownership across a fresh read: %s`,
    async (mode) => {
      const initial = initialCase();
      await render(form.Component, initial);
      const original = form.selectors.map((selector) => field(selector).value);
      const owned = form.selectors.map((_, index) => `自己的草稿 ${index}`);
      for (let index = 0; index < form.selectors.length; index++) {
        if (mode === "pristine") continue;
        await edit(form.selectors[index], mode === "cleared" ? "" : owned[index]);
        if (mode === "reverted") await edit(form.selectors[index], original[index]);
      }
      if (form.name === "rubric" && mode === "cleared") {
        await edit("#rubric-weight-c1", "");
        await act(async () => field("#rubric-evidence-c1").click());
      }
      const fresh = freshCase();
      fresh.rubric!.items[0].weight = 7;
      await render(form.Component, fresh);
      const latest =
        form.name === "prompt"
          ? [fresh.name, fresh.user_prompt]
          : form.name === "facts"
            ? [fresh.simulated_user_facts.join("\n")]
            : [fresh.rubric!.version, fresh.rubric!.items[0].text];
      expect(form.selectors.map((selector) => field(selector).value)).toEqual(
        mode === "pristine"
          ? latest
          : mode === "cleared"
            ? form.selectors.map(() => "")
            : mode === "reverted"
              ? original
              : owned,
      );
      if (form.name === "rubric" && mode === "cleared") {
        expect(field("#rubric-weight-c1").value).toBe("");
        expect((field("#rubric-evidence-c1") as HTMLInputElement).checked).toBe(false);
      }
    },
  );

  test.each(["success", "failure"] as const)(
    `${form.name} releases only successfully saved ownership: %s`,
    async (outcome) => {
      const writes: unknown[] = [];
      vi.stubGlobal("fetch", (_url: string, init?: RequestInit) => {
        expect(init?.method).toBe("PATCH");
        writes.push(JSON.parse(init!.body as string));
        return Promise.resolve(
          new Response(
            JSON.stringify(outcome === "success" ? freshCase() : { error: "更新失敗" }),
            {
              status: outcome === "success" ? 200 : 500,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      });
      await render(form.Component, initialCase());
      const owned = form.selectors.map((_, index) => `自己的草稿 ${index}`);
      for (let index = 0; index < form.selectors.length; index++) {
        await edit(form.selectors[index], owned[index]);
      }
      const save = container.querySelector<HTMLButtonElement>("button")!;
      expect(save.disabled).toBe(false);
      await act(async () => save.click());
      await pollUntil(
        () =>
          container.textContent!.includes(outcome === "success" ? "已儲存。" : "操作結果尚未確認"),
        () => container.textContent,
        DEFAULT_WAIT_MS,
      );
      const expectedPatch =
        form.name === "prompt"
          ? { name: owned[0], user_prompt: owned[1] }
          : form.name === "facts"
            ? { simulated_user_facts: owned }
            : {
                rubric: {
                  version: owned[0],
                  items: [{ ...TEST_CASE_DRAFT.rubric.items[0], text: owned[1] }],
                },
              };
      expect(writes).toEqual([expectedPatch]);
      const fresh = freshCase();
      await render(form.Component, fresh);
      const expected =
        form.name === "prompt"
          ? [fresh.name, fresh.user_prompt]
          : form.name === "facts"
            ? fresh.simulated_user_facts
            : [fresh.rubric!.version, fresh.rubric!.items[0].text];
      expect(form.selectors.map((selector) => field(selector).value)).toEqual(
        outcome === "success" ? expected : owned,
      );
    },
  );
}

test("editing a name leaves the untouched prompt owned by the latest read", async () => {
  await render(PromptForm, initialCase());
  await edit("#edit-name", "自己的名稱");
  await render(PromptForm, freshCase());
  expect(field("#edit-name").value).toBe("自己的名稱");
  expect(field("#edit-prompt").value).toBe("伺服器新版任務");
});

test("editing a prompt leaves the untouched name owned by the latest read", async () => {
  await render(PromptForm, initialCase());
  await edit("#edit-prompt", "自己的任務");
  await render(PromptForm, freshCase());
  expect(field("#edit-prompt").value).toBe("自己的任務");
  expect(field("#edit-name").value).toBe("伺服器新名稱");
});

test("editing one rubric field preserves freshly read version, weight and evidence", async () => {
  await render(RubricSection, initialCase());
  await edit("#rubric-c1", "自己的評分說明");
  const fresh = freshCase();
  fresh.rubric!.items[0].weight = 7;
  fresh.rubric!.items[0].evidence_required = false;
  await render(RubricSection, fresh);
  expect(field("#rubric-c1").value).toBe("自己的評分說明");
  expect(field("#rubric-version").value).toBe("server/v2");
  expect(field("#rubric-weight-c1").value).toBe("7");
  expect((field("#rubric-evidence-c1") as HTMLInputElement).checked).toBe(false);
});

test("an untouched rubric follows server creation and removal", async () => {
  const initial = initialCase();
  delete initial.rubric;
  await render(RubricSection, initial);
  expect(field("#rubric-version").value).toBe("");
  await render(RubricSection, freshCase());
  expect(field("#rubric-version").value).toBe("server/v2");
  expect(field("#rubric-c1").value).toBe("伺服器新評分說明");
  await render(RubricSection, initial);
  expect(field("#rubric-version").value).toBe("");
  expect(field("#rubric-c1").value).toBe("");
});
