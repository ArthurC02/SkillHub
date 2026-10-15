import { setImmediate } from "node:timers/promises";
import type { Page } from "@playwright/test";
import { afterEach, expect, test, vi } from "vitest";
import { setOffline } from "../../e2e/offline";

afterEach(() => {
  vi.unstubAllGlobals();
});

function networkPage(initiallyOnline: boolean, error?: Error) {
  let online = initiallyOnline;
  const events = new EventTarget();
  vi.stubGlobal("window", events);
  vi.stubGlobal("navigator", {
    get onLine() {
      return online;
    },
  });
  const dispose = vi.fn(async () => {});
  const emulate = vi.fn(async (offline: boolean) => {
    if (error) throw error;
    online = !offline;
  });
  const page = {
    context: () => ({ setOffline: emulate }),
    evaluateHandle: async (evaluate: (offline: boolean) => unknown, offline: boolean) => {
      const value = evaluate(offline);
      return {
        evaluate: async (evaluate: (value: unknown) => unknown) => evaluate(value),
        dispose,
      };
    },
  } as unknown as Page;
  return { page, events, emulate, dispose };
}

test.each([
  ["online to offline", true, true, false],
  ["offline to online", false, false, false],
  ["already online", true, false, true],
  ["already offline", false, true, true],
] as const)(
  "network transition %s waits only for a required browser event",
  async (_, online, offline, ready) => {
    const network = networkPage(online);
    let settled = false;
    const change = setOffline(network.page, offline).then(() => {
      settled = true;
    });
    try {
      await setImmediate();
      expect(network.emulate).toHaveBeenCalledExactlyOnceWith(offline);
      expect(settled).toBe(ready);
    } finally {
      network.events.dispatchEvent(new Event(offline ? "offline" : "online"));
      await change;
    }
    expect(settled).toBe(true);
  },
);

test("a rejected network transition releases its event handle and retains the error", async () => {
  const error = new Error("emulation unavailable");
  const network = networkPage(true, error);
  await expect(setOffline(network.page, true)).rejects.toBe(error);
  expect(network.dispose).toHaveBeenCalledOnce();
});
