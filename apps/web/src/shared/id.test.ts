import { afterEach, expect, test, vi } from "vitest";
import { newClientID } from "./id";

afterEach(() => vi.unstubAllGlobals());

test("client ID uses the browser UUID generator when available", () => {
  vi.stubGlobal("crypto", { randomUUID: () => "12345678-1234-4234-8234-123456789abc" });
  expect(newClientID()).toBe("12345678-1234-4234-8234-123456789abc");
});

test("client ID remains a version-four UUID when randomUUID is unavailable", () => {
  vi.stubGlobal("crypto", {
    getRandomValues: (bytes: Uint8Array) => {
      bytes.forEach((_, index) => (bytes[index] = index));
      return bytes;
    },
  });
  expect(newClientID()).toBe("00010203-0405-4607-8809-0a0b0c0d0e0f");
});
