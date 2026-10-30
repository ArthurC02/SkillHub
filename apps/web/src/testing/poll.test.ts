import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { pollUntil, PollOutlivedItsTest } from "./poll";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(0);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

test.each([false, true])(
  "a condition completed by the final flush succeeds with flushBeforeFirstCheck=%s",
  async (flushBeforeFirstCheck) => {
    let ready = false;
    setTimeout(() => {
      ready = true;
    }, 2);
    const result = pollUntil(
      () => ready,
      () => "ready",
      5,
      { flushBeforeFirstCheck },
    );
    const assertion = expect(result).resolves.toBeUndefined();

    await vi.advanceTimersByTimeAsync(5);
    await assertion;
    expect(Date.now()).toBe(5);
    expect(vi.getTimerCount()).toBe(0);
  },
);

test("an already satisfied condition needs no flush by default", async () => {
  await expect(
    pollUntil(
      () => true,
      () => "ready",
      5,
    ),
  ).resolves.toBeUndefined();
  expect(Date.now()).toBe(0);
  expect(vi.getTimerCount()).toBe(0);
});

test("flushBeforeFirstCheck flushes before evaluating an already satisfied condition", async () => {
  const done = vi.fn(() => true);
  const result = pollUntil(done, () => "ready", 5, { flushBeforeFirstCheck: true });
  expect(done).not.toHaveBeenCalled();

  await vi.advanceTimersByTimeAsync(5);
  await expect(result).resolves.toBeUndefined();
  expect(done).toHaveBeenCalledOnce();
  expect(Date.now()).toBe(5);
  expect(vi.getTimerCount()).toBe(0);
});

test.each([false, true])(
  "a condition still false after the final flush times out with flushBeforeFirstCheck=%s",
  async (flushBeforeFirstCheck) => {
    const result = pollUntil(
      () => false,
      () => "still loading",
      5,
      { flushBeforeFirstCheck },
    );
    const assertion = expect(result).rejects.toThrow("waitFor timed out; DOM was: still loading");

    await vi.advanceTimersByTimeAsync(5);
    await assertion;
    expect(vi.getTimerCount()).toBe(0);
  },
);

test.each([false, true])(
  "a poll whose owner changed during a flush stops with flushBeforeFirstCheck=%s",
  async (flushBeforeFirstCheck) => {
    const state = expect.getState();
    let currentTestName = state.currentTestName;
    const getState = vi
      .spyOn(expect, "getState")
      .mockImplementation(() => ({ ...state, currentTestName }));
    let ready = false;
    setTimeout(() => {
      currentTestName = "another test";
      ready = true;
    }, 2);
    const result = pollUntil(
      () => ready,
      () => "ready",
      5,
      { flushBeforeFirstCheck },
    );
    const assertion = expect(result).rejects.toBeInstanceOf(PollOutlivedItsTest);

    try {
      await vi.advanceTimersByTimeAsync(5);
      await assertion;
    } finally {
      getState.mockRestore();
    }
    expect(vi.getTimerCount()).toBe(0);
  },
);
