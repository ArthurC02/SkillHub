import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { expect, test, type TestInfo } from "@playwright/test";
import { captureSocketFailures, socketFailures } from "./socket-diagnostics";

test("socket diagnostics retain native error codes and exclude unapproved trace content", () => {
  const secret = "fixture-secret";
  expect(
    socketFailures({
      traceEvents: [
        {
          name: "TCP_CONNECT",
          ts: 10,
          args: { params: { net_error: -176, url: secret, headers: { authorization: secret } } },
        },
        {
          name: "TCP_CONNECT_ATTEMPT",
          ts: 11,
          args: { params: { os_error: 10055, address: "127.0.0.1:4173", body: secret } },
        },
        {
          name: "TCP_CONNECT_ATTEMPT",
          ts: 12,
          args: { params: { os_error: 111, address: "[::1]:65535" } },
        },
        {
          name: "HTTP_TRANSACTION_SEND_REQUEST_HEADERS",
          ts: 13,
          args: { params: { net_error: -176, headers: secret } },
        },
        { name: "TCP_CONNECT", ts: 14, args: { params: { address: "127.0.0.1:4173" } } },
        null,
      ],
    }),
  ).toEqual([
    { event: "TCP_CONNECT", time_us: 10, net_error: -176 },
    {
      event: "TCP_CONNECT_ATTEMPT",
      time_us: 11,
      os_error: 10055,
      loopback_address: "127.0.0.1:4173",
    },
    { event: "TCP_CONNECT_ATTEMPT", time_us: 12, os_error: 111, loopback_address: "[::1]:65535" },
  ]);
});

for (const address of [
  "127.0.0.1:0",
  "127.0.0.1:65536",
  "127.0.0.1:4173/fixture-secret",
  "fixture-secret:4173",
  "192.0.2.1:4173",
]) {
  test(`socket diagnostics exclude an unapproved address ${address}`, () => {
    expect(
      socketFailures({
        traceEvents: [
          { name: "TCP_CONNECT_ATTEMPT", ts: -1, args: { params: { os_error: 10055, address } } },
        ],
      }),
    ).toEqual([{ event: "TCP_CONNECT_ATTEMPT", os_error: 10055 }]);
  });
}

test("socket diagnostics reject a missing trace and distinguish an empty observation", () => {
  for (const trace of [null, {}, { traceEvents: null }])
    expect(() => socketFailures(trace)).toThrow("Socket trace events are unavailable");
  expect(socketFailures({ traceEvents: [] })).toEqual([]);
});

test("socket diagnostics omit invalid error codes and retain the lowest valid loopback port", () => {
  expect(
    socketFailures({
      traceEvents: [
        ...[0, 1, "-176", null, Number.NaN, Number.NEGATIVE_INFINITY].map((net_error) => ({
          name: "TCP_CONNECT",
          args: { params: { net_error } },
        })),
        ...[0, -1, "10055", null, 1.5, Number.POSITIVE_INFINITY].map((os_error) => ({
          name: "TCP_CONNECT_ATTEMPT",
          args: { params: { os_error } },
        })),
        {
          name: "TCP_CONNECT_ATTEMPT",
          args: { params: { os_error: 10055, address: "127.0.0.1:1" } },
        },
      ],
    }),
  ).toEqual([{ event: "TCP_CONNECT_ATTEMPT", os_error: 10055, loopback_address: "127.0.0.1:1" }]);
});

for (const [status, expectedStatus, attached] of [
  ["passed", "passed", false],
  ["failed", "passed", true],
  ["failed", "failed", false],
  ["passed", "failed", true],
] as const) {
  test(`socket tracing stops once for ${status} with expected ${expectedStatus} and attachment ${attached}`, async () => {
    const calls: string[] = [];
    const failure = new Error("original assertion");
    const browser = {
      startTracing: async () => {
        calls.push("start");
      },
      stopTracing: async () => {
        calls.push("stop");
        return Buffer.from('{"traceEvents":[]}');
      },
    };
    const info = {
      status,
      expectedStatus,
      attach: async (name: string, options?: { body?: string | Buffer; contentType?: string }) => {
        calls.push(name);
        expect(options).toEqual({ body: "[]", contentType: "application/json" });
      },
    };
    const operation = captureSocketFailures(
      browser,
      "chromium",
      async () => {
        calls.push("test");
        if (status === "failed") throw failure;
      },
      info,
    );
    if (status === "failed") await expect(operation).rejects.toBe(failure);
    else await operation;
    expect(calls).toEqual(
      attached ? ["start", "test", "stop", "socket-failures"] : ["start", "test", "stop"],
    );
  });
}

test("engines without Chromium tracing run the original test once without a recorder", async ({
  browserName,
}) => {
  let calls = 0;
  let recorderCalls = 0;
  let attachments = 0;
  const unsupported = {
    startTracing: async () => {
      recorderCalls++;
    },
    stopTracing: async () => {
      recorderCalls++;
      return Buffer.from('{"traceEvents":[]}');
    },
  };
  const info = {
    status: "failed" as const,
    expectedStatus: "passed" as const,
    attach: async () => {
      attachments++;
    },
  };
  const engine = browserName === "chromium" ? "firefox" : browserName;
  await captureSocketFailures(
    unsupported,
    engine,
    async () => {
      calls++;
    },
    info,
  );
  expect(calls).toBe(1);
  expect(recorderCalls).toBe(0);
  expect(attachments).toBe(0);
});

test("native refused navigation remains an error and socket attachments contain no fixture credentials", async ({
  browser,
  browserName,
}) => {
  const secret = "fixture-only-socket-secret";
  const server = createServer((_request, response) => {
    response.writeHead(200, {
      "content-type": "text/html",
      "set-cookie": `probe=${secret}; HttpOnly`,
    });
    response.end("<!doctype html><title>Socket probe</title>");
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as AddressInfo).port;
  const context = await browser.newContext({
    extraHTTPHeaders: { Authorization: `Bearer ${secret}` },
  });
  const attachments: string[] = [];
  const info: Pick<TestInfo, "status" | "expectedStatus" | "attach"> = {
    status: "passed",
    expectedStatus: "passed",
    attach: async (_name, options) => {
      attachments.push(String(options?.body));
    },
  };
  let navigations = 0;
  let navigationError: unknown;
  try {
    await captureSocketFailures(
      browser,
      browserName,
      async () => {
        const initial = await context.newPage();
        expect((await initial.goto(`http://127.0.0.1:${port}/`))?.status()).toBe(200);
        await initial.close();
        server.closeAllConnections();
        await new Promise<void>((resolve) => server.close(() => resolve()));
        const page = await context.newPage();
        navigations++;
        try {
          await page.goto(`http://127.0.0.1:${port}/`);
        } catch (error) {
          navigationError = error;
          info.status = "failed";
          throw error;
        }
      },
      info,
    ).catch((error) => {
      expect(error).toBe(navigationError);
    });
    expect(navigationError).toBeInstanceOf(Error);
    expect(navigations).toBe(1);
    if (browserName === "chromium") {
      expect(attachments).toHaveLength(1);
      expect(attachments[0]).not.toContain(secret);
      expect(attachments[0]).not.toContain("Authorization");
      const failures = JSON.parse(attachments[0]!);
      expect(failures).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ event: "TCP_CONNECT", net_error: -102 }),
          expect.objectContaining({
            event: "TCP_CONNECT_ATTEMPT",
            loopback_address: `127.0.0.1:${port}`,
            os_error: process.platform === "win32" ? 10061 : 111,
          }),
        ]),
      );
    } else expect(attachments).toEqual([]);
  } finally {
    await context.close();
    if (server.listening) {
      server.closeAllConnections();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  }
});
