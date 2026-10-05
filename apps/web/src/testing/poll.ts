import { act } from "react";
import { expect } from "vitest";

const POLL_INTERVAL_MS = 5;

export const DEFAULT_WAIT_MS = 10_000;

export class PollOutlivedItsTest extends Error {}

const runningTest = () => expect.getState().currentTestName;

const flushOnce = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
  });

// A test that times out keeps running in the background; stopping its poll as
// soon as another test has started keeps its act() calls out of that test.
export async function pollUntil(
  done: () => boolean,
  describe: () => string | null | undefined,
  timeoutMs: number,
  { flushBeforeFirstCheck = false } = {},
) {
  const owner = runningTest();
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (runningTest() !== owner) throw new PollOutlivedItsTest(`poll from "${owner}" outlived it`);
    if (!flushBeforeFirstCheck && done()) return;
    await flushOnce();
    if (flushBeforeFirstCheck && done()) return;
  }
  throw new Error(`waitFor timed out; DOM was: ${describe() ?? ""}`);
}
