import { act, StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { createMemoryHistory } from "@tanstack/react-router";
import axe from "axe-core";
import { afterEach, beforeAll, beforeEach, expect, vi } from "vitest";
import App from "../app/App";
import { createAppRouter } from "../app/router";
import { queryClient } from "../core/api/queryClient";
import { platformResponse } from "./fixtures/platform";
import { preloadEveryPage } from "./pages";
import { DEFAULT_WAIT_MS, pollUntil } from "./poll";

export type Reply = { status?: number; json?: unknown } | "network-error";
export type Request = { url: URL; method: string; body: unknown };
type Handler = (request: Request) => Reply | undefined | Promise<Reply | undefined>;
type Matcher = string | RegExp | ((url: URL) => boolean);

const AXE_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"];
const LAYOUT_RULES = { "color-contrast": { enabled: false }, region: { enabled: false } };

function globToRegExp(glob: string): RegExp {
  let source = "";
  for (let i = 0; i < glob.length; i += 1) {
    const char = glob[i];
    if (char === "*" && glob[i + 1] === "*") {
      source += ".*";
      i += 1;
    } else if (char === "*") source += "[^/]*";
    else source += char.replace(/[.+?^${}()|[\]\\/]/g, "\\$&");
  }
  return new RegExp(`^${source}$`);
}

function matches(matcher: Matcher, url: URL): boolean {
  if (typeof matcher === "function") return matcher(url);
  const href = url.href;
  if (matcher instanceof RegExp) return matcher.test(href);
  return globToRegExp(matcher).test(href);
}

const BODILESS_STATUSES = new Set([204, 205, 304]);

function jsonResponse(reply: Exclude<Reply, "network-error">): Response {
  const status = reply.status ?? 200;
  if (BODILESS_STATUSES.has(status)) return new Response(null, { status });
  return new Response(reply.json === undefined ? "{}" : JSON.stringify(reply.json), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function collapse(text: string | null | undefined): string {
  return (text ?? "").replace(/\s+/g, " ").trim();
}

function accessibleName(element: Element): string {
  const labelledBy = element.getAttribute("aria-labelledby");
  if (labelledBy) {
    return collapse(
      labelledBy
        .split(/\s+/)
        .map((id) => element.ownerDocument.getElementById(id)?.textContent ?? "")
        .join(" "),
    );
  }
  const label = element.getAttribute("aria-label");
  if (label) return collapse(label);
  if (
    element instanceof HTMLInputElement ||
    element instanceof HTMLTextAreaElement ||
    element instanceof HTMLSelectElement
  ) {
    const owned = element.labels?.[0];
    if (owned) return collapse(owned.textContent);
    if (element instanceof HTMLInputElement && ["button", "submit"].includes(element.type))
      return collapse(element.value);
    return collapse(element.getAttribute("placeholder") ?? element.getAttribute("title"));
  }
  if (element.tagName === "IMG") return collapse(element.getAttribute("alt"));
  return collapse(element.textContent || element.getAttribute("title"));
}

const ROLE_SELECTORS: Record<string, string> = {
  alert: "[role=alert]",
  button: "button, [role=button], input[type=button], input[type=submit]",
  checkbox: "input[type=checkbox], [role=checkbox]",
  combobox: "select, [role=combobox]",
  dialog: "dialog, [role=dialog], [role=alertdialog]",
  heading: "h1, h2, h3, h4, h5, h6, [role=heading]",
  link: "a[href], [role=link]",
  list: "ul, ol, [role=list]",
  listitem: "li, [role=listitem]",
  navigation: "nav, [role=navigation]",
  radio: "input[type=radio], [role=radio]",
  region: "section[aria-label], section[aria-labelledby], [role=region]",
  status: "[role=status], output",
  tab: "[role=tab]",
  tabpanel: "[role=tabpanel]",
  textbox:
    "textarea, input:not([type]), input[type=text], input[type=search], input[type=email], input[type=url], input[type=number], [role=textbox]",
};

function isHidden(element: Element): boolean {
  for (let node: Element | null = element; node; node = node.parentElement) {
    if (node.hasAttribute("hidden")) return true;
    if (getComputedStyle(node).display === "none") return true;
    const details = node.parentElement;
    if (
      details?.tagName === "DETAILS" &&
      !details.hasAttribute("open") &&
      node.tagName !== "SUMMARY"
    )
      return true;
  }
  return false;
}

type TextQuery = string | RegExp;

function implicitlySubmit(input: HTMLElement) {
  const form = (input as HTMLInputElement).form;
  if (!form) return;
  const submitter = form.querySelector<HTMLButtonElement | HTMLInputElement>(
    "button:not([type]), button[type=submit], input[type=submit]",
  );
  if (submitter?.disabled) return;
  form.requestSubmit(submitter ?? undefined);
}

async function pressOn(target: HTMLElement, key: string) {
  await act(async () => {
    const proceed = target.dispatchEvent(
      new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true }),
    );
    if (proceed && key === "Enter") {
      if (target.tagName === "BUTTON" || target.tagName === "A") target.click();
      else if (target.tagName === "INPUT") implicitlySubmit(target);
    }
    target.dispatchEvent(new KeyboardEvent("keyup", { key, bubbles: true }));
  });
}

function textMatches(actual: string, expected: TextQuery, exact: boolean): boolean {
  if (expected instanceof RegExp) return expected.test(actual);
  return exact ? actual === expected : actual.toLowerCase().includes(expected.toLowerCase());
}

export class Locator {
  private readonly resolve: () => Element[];

  constructor(resolve: () => Element[]) {
    this.resolve = resolve;
  }

  all(): Element[] {
    return this.resolve().filter((element) => !isHidden(element));
  }

  count(): number {
    return this.all().length;
  }

  first(): Locator {
    return new Locator(() => this.all().slice(0, 1));
  }

  nth(index: number): Locator {
    return new Locator(() => this.all().slice(index, index + 1));
  }

  filter({ hasText }: { hasText: TextQuery }): Locator {
    return new Locator(() =>
      this.all().filter((element) => textMatches(collapse(element.textContent), hasText, false)),
    );
  }

  locator(selector: string): Locator {
    return new Locator(() =>
      this.all().flatMap((element) => [...element.querySelectorAll(selector)]),
    );
  }

  element(): HTMLElement {
    const found = this.all();
    if (found.length !== 1) throw new Error(`expected exactly one element, found ${found.length}`);
    return found[0] as HTMLElement;
  }

  text(): string {
    return this.all()
      .map((element) => collapse(element.textContent))
      .join(" ");
  }

  attribute(name: string): string | null {
    return this.element().getAttribute(name);
  }

  value(): string {
    return (this.element() as HTMLInputElement).value;
  }

  async press(key: string): Promise<void> {
    await this.focus();
    await pressOn(this.element(), key);
  }

  isDisabled(): boolean {
    const element = this.element();
    return element.hasAttribute("disabled") || element.getAttribute("aria-disabled") === "true";
  }

  isFocused(): boolean {
    const found = this.all();
    return found.length === 1 && found[0] === document.activeElement;
  }

  async click(): Promise<void> {
    const element = this.element();
    await act(async () => {
      element.focus();
      element.click();
    });
  }

  async focus(): Promise<void> {
    const element = this.element();
    await act(async () => element.focus());
  }

  async fill(value: string): Promise<void> {
    const element = this.element() as HTMLInputElement | HTMLTextAreaElement;
    const prototype = Object.getPrototypeOf(element) as object;
    const setter = Object.getOwnPropertyDescriptor(prototype, "value")?.set;
    await act(async () => {
      element.focus();
      setter?.call(element, value);
      element.dispatchEvent(new Event("input", { bubbles: true }));
      element.dispatchEvent(new Event("change", { bubbles: true }));
    });
  }

  async check(): Promise<void> {
    const element = this.element() as HTMLInputElement;
    if (!element.checked) await this.click();
  }

  async select(value: string): Promise<void> {
    const element = this.element() as HTMLSelectElement;
    await act(async () => {
      element.value = value;
      element.dispatchEvent(new Event("change", { bubbles: true }));
    });
  }
}

export type App = ReturnType<typeof createHarness>;

type Network = {
  handlers: Array<{ matcher: Matcher; handler: Handler }>;
  requests: Request[];
  offline: boolean;
};

function parsedBody(body: unknown): unknown {
  if (typeof body !== "string") return body;
  try {
    return JSON.parse(body);
  } catch {
    return body;
  }
}

function stubPlatformFetch(network: Network, router: ReturnType<typeof createAppRouter>) {
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit): Promise<Response> => {
    const url = new URL(String(input), "http://127.0.0.1");
    const request = {
      url,
      method: (init?.method ?? "GET").toUpperCase(),
      body: parsedBody(init?.body),
    };
    network.requests.push(request);
    if (network.offline) throw new TypeError("Failed to fetch");
    for (let i = network.handlers.length - 1; i >= 0; i -= 1) {
      const { matcher, handler } = network.handlers[i];
      if (!matches(matcher, url)) continue;
      const answer = await handler(request);
      if (answer === undefined) continue;
      if (answer === "network-error") throw new TypeError("Failed to fetch");
      return jsonResponse(answer);
    }
    const fallback = platformResponse(url.pathname + url.search);
    const operatorPage = router.state.location.pathname.startsWith("/admin");
    const json =
      operatorPage && url.pathname === "/me"
        ? { ...(fallback.body as object), operator: true }
        : fallback.body;
    return jsonResponse({ status: fallback.status, json });
  });
}

function domQueries(scope: () => HTMLElement | undefined) {
  const query = (selector: string) => () => [...(scope()?.querySelectorAll(selector) ?? [])];
  return {
    locator: (selector: string) => new Locator(query(selector)),
    getByRole(role: string, options: { name?: TextQuery; exact?: boolean } = {}) {
      const selector = ROLE_SELECTORS[role];
      if (!selector) throw new Error(`no selector for role ${role}`);
      return new Locator(() =>
        query(selector)().filter(
          (element) =>
            options.name === undefined ||
            textMatches(accessibleName(element), options.name, options.exact ?? false),
        ),
      );
    },
    getByText(text: TextQuery, { exact = false }: { exact?: boolean } = {}) {
      return new Locator(() => {
        const hits = query("*")().filter((element) =>
          textMatches(collapse(element.textContent), text, exact),
        );
        return hits.filter(
          (element) => !hits.some((other) => other !== element && element.contains(other)),
        );
      });
    },
    getByLabel(text: TextQuery, { exact = false }: { exact?: boolean } = {}) {
      return new Locator(() =>
        query("input, textarea, select, [role=textbox], [role=combobox]")().filter((element) =>
          textMatches(accessibleName(element), text, exact),
        ),
      );
    },
  };
}

async function axeViolations(scope: Element): Promise<string[]> {
  const results = await axe.run(scope, {
    runOnly: { type: "tag", values: AXE_TAGS },
    rules: LAYOUT_RULES,
    resultTypes: ["violations"],
  });
  return results.violations.map(
    (v) => `${v.id}: ${v.nodes.map((n) => n.html.slice(0, 120)).join(" | ")}`,
  );
}

function eventually(assertion: () => void, describe: () => string, timeoutMs = DEFAULT_WAIT_MS) {
  let last: unknown;
  return pollUntil(
    () => {
      try {
        assertion();
        return true;
      } catch (error) {
        last = error;
        return false;
      }
    },
    () => `${last instanceof Error ? last.message : String(last)}
--- DOM: ${describe()}`,
    timeoutMs,
  );
}

function setNavigatorOnline(online: boolean) {
  Object.defineProperty(navigator, "onLine", { configurable: true, get: () => online });
}

function createHarness() {
  let root: Root | undefined;
  let container: HTMLDivElement | undefined;
  const network: Network = { handlers: [], requests: [], offline: false };
  const router = createAppRouter();
  stubPlatformFetch(network, router);

  async function mount(path: string) {
    router.update({ history: createMemoryHistory({ initialEntries: [path] }) });
    const host = document.createElement("div");
    container = host;
    document.body.appendChild(host);
    await act(async () => {
      root = createRoot(host);
      root.render(
        <StrictMode>
          <App router={router} />
        </StrictMode>,
      );
    });
  }

  return {
    requests: network.requests,
    ...domQueries(() => container),
    route(matcher: Matcher, handler: Handler) {
      network.handlers.push({ matcher, handler });
    },
    async goto(path: string) {
      if (!root) return mount(path);
      await act(async () => {
        await router.navigate({ href: path });
      });
    },
    async goBack() {
      await act(async () => router.history.back());
    },
    pathname: () => router.state.location.pathname,
    search: () => router.state.location.searchStr,
    async press(key: string) {
      const target = document.activeElement as HTMLElement | null;
      if (!target || target === document.body) throw new Error("nothing is focused");
      await pressOn(target, key);
    },
    async setOffline(value: boolean) {
      network.offline = value;
      setNavigatorOnline(!value);
      await act(async () => {
        window.dispatchEvent(new Event(value ? "offline" : "online"));
      });
    },
    eventually: (assertion: () => void, timeoutMs?: number) =>
      eventually(assertion, () => container?.textContent ?? "", timeoutMs),
    axeViolations(selector = "main") {
      const scope = container?.querySelector(selector) ?? container;
      if (!scope) throw new Error("nothing is rendered");
      return axeViolations(scope);
    },
    async unmount() {
      await act(async () => root?.unmount());
      container?.remove();
      setNavigatorOnline(true);
    },
  };
}

let current: App | undefined;

export function renderedApp(): () => App {
  beforeAll(preloadEveryPage);
  beforeEach(() => {
    queryClient.clear();
    current = createHarness();
  });
  afterEach(async () => {
    await current?.unmount();
    current = undefined;
    vi.unstubAllGlobals();
  });
  return () => {
    if (!current) throw new Error("renderedApp() is read outside a test");
    return current;
  };
}

export { expect };
