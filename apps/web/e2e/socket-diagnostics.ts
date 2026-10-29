import { test as base, type Browser, type TestInfo } from "@playwright/test";

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function loopbackAddress(value: unknown) {
  if (typeof value !== "string") return undefined;
  const match = /^(127\.0\.0\.1|\[::1\]):([0-9]{1,5})$/.exec(value);
  const port = match ? Number(match[2]) : 0;
  return port > 0 && port <= 65535 ? value : undefined;
}

export function socketFailures(trace: unknown) {
  if (!record(trace) || !Array.isArray(trace.traceEvents))
    throw new Error("Socket trace events are unavailable");
  return trace.traceEvents.flatMap((event: unknown) => {
    if (!record(event) || (event.name !== "TCP_CONNECT" && event.name !== "TCP_CONNECT_ATTEMPT"))
      return [];
    const params = record(event.args) && record(event.args.params) ? event.args.params : {};
    const netError =
      typeof params.net_error === "number" &&
      Number.isSafeInteger(params.net_error) &&
      params.net_error < 0;
    const osError =
      typeof params.os_error === "number" &&
      Number.isSafeInteger(params.os_error) &&
      params.os_error > 0;
    if (!netError && !osError) return [];
    const address = loopbackAddress(params.address);
    return [
      {
        event: event.name,
        ...(typeof event.ts === "number" && Number.isFinite(event.ts) && event.ts >= 0
          ? { time_us: event.ts }
          : {}),
        ...(netError ? { net_error: params.net_error } : {}),
        ...(osError ? { os_error: params.os_error } : {}),
        ...(address ? { loopback_address: address } : {}),
      },
    ];
  });
}

export async function captureSocketFailures(
  browser: Pick<Browser, "startTracing" | "stopTracing">,
  browserName: string,
  runTest: () => Promise<void>,
  info: Pick<TestInfo, "status" | "expectedStatus" | "attach">,
) {
  if (browserName !== "chromium") return runTest();
  await browser.startTracing(undefined, { categories: ["netlog"], screenshots: false });
  try {
    await runTest();
  } finally {
    const trace = await browser.stopTracing();
    if (info.status !== info.expectedStatus)
      await info.attach("socket-failures", {
        body: JSON.stringify(socketFailures(JSON.parse(trace.toString()))),
        contentType: "application/json",
      });
  }
}

export const test = base.extend<{ socketDiagnostics: void }>({
  socketDiagnostics: [
    async ({ browser, browserName }, use, info) => {
      await captureSocketFailures(browser, browserName, use, info);
    },
    { auto: true },
  ],
});
