import { StrictMode, act, useEffect, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ActionForm } from "./ActionForm";
import { WriteFailure } from "./WriteFailure";

let container: HTMLDivElement;
let root: Root;
let scrolled: Element[];
let originalScroll: typeof HTMLElement.prototype.scrollIntoView | undefined;

beforeEach(() => {
  scrolled = [];
  originalScroll = HTMLElement.prototype.scrollIntoView;
  HTMLElement.prototype.scrollIntoView = function () {
    scrolled.push(this);
  };
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
  if (originalScroll) HTMLElement.prototype.scrollIntoView = originalScroll;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

async function type(selector: string, value: string) {
  const input = container.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector)!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(input), "value")!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  return input;
}

for (const scenario of [
  { name: "unmanaged", reveal: undefined, first: 1, next: 2 },
  { name: "parent-managed", reveal: false, first: 0, next: 0 },
]) {
  test(`WriteFailure reveals each new ${scenario.name} failure only under its owner`, async () => {
    const render = (error: unknown) =>
      act(async () => {
        root.render(
          <StrictMode>
            <WriteFailure error={error} reveal={scenario.reveal} />
          </StrictMode>,
        );
      });
    await render(undefined);
    expect(scrolled).toHaveLength(0);
    const error = new Error("first unknown result");
    await render(error);
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(
      "無法確認操作是否完成；請先重新整理目前狀態，再決定是否重試。",
    );
    expect(scrolled).toHaveLength(scenario.first);
    await render(error);
    expect(scrolled).toHaveLength(scenario.first);
    await render(new Error("next unknown result"));
    expect(scrolled).toHaveLength(scenario.next);
  });
}

for (const outcome of ["error", "success"] as const) {
  for (const owner of ["submitted form", "another control", "new draft"] as const) {
    test(`ActionForm follows each ${outcome} result while respecting focus on ${owner}`, async () => {
      let respond!: () => void;
      function Harness() {
        const [subject, setSubject] = useState("first decision");
        const [pending, setPending] = useState(false);
        const [error, setError] = useState<unknown>();
        const [done, setDone] = useState<string>();
        useEffect(() => {
          respond = () => {
            setPending(false);
            if (outcome === "error") setError(new Error("outcome unavailable"));
            else setDone("動作已完成。");
          };
        });
        return (
          <>
            <ActionForm
              id="decision"
              submitLabel="確認"
              contextKey={subject}
              pending={pending}
              error={error}
              done={done}
              onSubmit={() => {
                setError(undefined);
                setDone(undefined);
                setPending(true);
              }}
            >
              <input
                id="subject"
                value={subject}
                onChange={(event) => setSubject(event.target.value)}
              />
            </ActionForm>
            <button id="other" type="button">
              其他操作
            </button>
          </>
        );
      }
      await act(async () =>
        root.render(
          <StrictMode>
            <Harness />
          </StrictMode>,
        ),
      );
      await type("#decision-note", "verified reason");
      const submit = container.querySelector<HTMLButtonElement>('button[type="submit"]')!;
      const send = () =>
        act(async () => {
          submit.focus();
          submit.click();
        });
      await send();
      expect(submit.disabled).toBe(true);
      expect(document.activeElement).toBe(submit);
      let preserved: HTMLElement | undefined;
      if (owner === "another control") {
        preserved = container.querySelector<HTMLElement>("#other")!;
        await act(async () => preserved!.focus());
      } else if (owner === "new draft") {
        preserved = container.querySelector<HTMLElement>("#subject")!;
        await act(async () => preserved!.focus());
        await type("#subject", "next decision");
      }
      await act(async () => respond());
      const selector = outcome === "error" ? '[role="alert"]' : '[role="status"]';
      if (owner === "new draft") {
        expect(container.querySelector(selector)).toBeNull();
        expect(document.activeElement).toBe(preserved);
        expect(scrolled).toHaveLength(0);
        return;
      }
      const expected =
        outcome === "error"
          ? "無法確認操作是否完成；請先重新整理目前狀態，再決定是否重試。"
          : "動作已完成。";
      expect(container.querySelector(selector)?.textContent).toBe(expected);
      if (owner === "another control") {
        expect(document.activeElement).toBe(preserved);
        expect(scrolled).toHaveLength(0);
        return;
      }
      expect(document.activeElement?.contains(container.querySelector(selector))).toBe(true);
      expect(scrolled).toHaveLength(1);
      expect(scrolled[0]).toBe(document.activeElement);
      await type("#subject", "second decision");
      await send();
      await act(async () => respond());
      expect(container.querySelector(selector)?.textContent).toBe(expected);
      expect(document.activeElement?.contains(container.querySelector(selector))).toBe(true);
      expect(scrolled).toHaveLength(2);
    });
  }
}
