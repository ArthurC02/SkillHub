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

export type Reply =
  { status?: number; json?: unknown; body?: string; contentType?: string } | "network-error";
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
  if (reply.body !== undefined)
    return new Response(reply.body, {
      status,
      headers: { "Content-Type": reply.contentType ?? "application/json" },
    });
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
  group: "fieldset, [role=group]",
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
  table: "table, [role=table]",
  row: "tr, [role=row]",
  cell: "td, [role=cell]",
  columnheader: "th[scope=col], thead th, [role=columnheader]",
  rowheader: "th[scope=row], tbody th, [role=rowheader]",
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
      if (["BUTTON", "A", "SUMMARY"].includes(target.tagName)) target.click();
      else if (target.tagName === "INPUT") implicitlySubmit(target);
    }
    const toggles =
      target.tagName === "BUTTON" ||
      (target instanceof HTMLInputElement && ["checkbox", "radio"].includes(target.type));
    if (proceed && key === " " && toggles) target.click();
    target.dispatchEvent(new KeyboardEvent("keyup", { key, bubbles: true }));
  });
}

function textMatches(actual: string, expected: TextQuery, exact: boolean): boolean {
  if (expected instanceof RegExp) return expected.test(actual);
  return exact ? actual === expected : actual.toLowerCase().includes(expected.toLowerCase());
}

type RoleQuery = { name?: TextQuery; exact?: boolean };

function isDisabledElement(element: Element): boolean {
  return element.hasAttribute("disabled") || element.getAttribute("aria-disabled") === "true";
}

function byLabel(scope: ParentNode, text: TextQuery, { exact = false }: { exact?: boolean }) {
  return [
    ...scope.querySelectorAll("input, textarea, select, [role=textbox], [role=combobox]"),
  ].filter((element) => textMatches(accessibleName(element), text, exact));
}

function byRole(scope: ParentNode, role: string, options: RoleQuery): Element[] {
  const selector = ROLE_SELECTORS[role];
  if (!selector) throw new Error(`no selector for role ${role}`);
  return [...scope.querySelectorAll(selector)].filter(
    (element) =>
      options.name === undefined ||
      textMatches(accessibleName(element), options.name, options.exact ?? false),
  );
}

function byText(scope: ParentNode, text: TextQuery, { exact = false }: { exact?: boolean }) {
  const hits = [...scope.querySelectorAll("*")].filter((element) =>
    textMatches(collapse(element.textContent), text, exact),
  );
  return hits.filter(
    (element) => !hits.some((other) => other !== element && element.contains(other)),
  );
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

  last(): Locator {
    return new Locator(() => this.all().slice(-1));
  }

  filter({ hasText, has }: { hasText?: TextQuery; has?: Locator }): Locator {
    return new Locator(() =>
      this.all().filter(
        (element) =>
          (hasText === undefined || textMatches(collapse(element.textContent), hasText, false)) &&
          (has === undefined || has.all().some((inner) => element.contains(inner))),
      ),
    );
  }

  hiddenElement(): HTMLElement {
    const found = this.resolve();
    if (found.length !== 1) throw new Error(`expected exactly one element, found ${found.length}`);
    return found[0] as HTMLElement;
  }

  locator(selector: string): Locator {
    return new Locator(() =>
      this.all().flatMap((element) => [...element.querySelectorAll(selector)]),
    );
  }

  getByRole(role: string, options: RoleQuery = {}): Locator {
    return new Locator(() => this.all().flatMap((element) => byRole(element, role, options)));
  }

  getByText(text: TextQuery, options: { exact?: boolean } = {}): Locator {
    return new Locator(() => this.all().flatMap((element) => byText(element, text, options)));
  }

  getByLabel(text: TextQuery, options: { exact?: boolean } = {}): Locator {
    return new Locator(() => this.all().flatMap((element) => byLabel(element, text, options)));
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

  private async actionable({ enabled }: { enabled: boolean }): Promise<HTMLElement> {
    let found: Element[] = [];
    const ready = () => {
      found = this.all();
      if (found.length !== 1) return false;
      return !enabled || !isDisabledElement(found[0]);
    };
    await pollUntil(
      ready,
      () =>
        found.length === 1
          ? "the element stayed disabled"
          : `expected exactly one element, found ${found.length}`,
      DEFAULT_WAIT_MS,
    );
    return found[0] as HTMLElement;
  }

  async press(key: string): Promise<void> {
    const element = await this.actionable({ enabled: false });
    await act(async () => element.focus());
    await pressOn(element, key);
  }

  isDisabled(): boolean {
    return isDisabledElement(this.element());
  }

  isFocused(): boolean {
    const found = this.all();
    return found.length === 1 && found[0] === document.activeElement;
  }

  async click(): Promise<void> {
    const element = await this.actionable({ enabled: true });
    await act(async () => {
      element.focus();
      element.click();
    });
  }

  async focus(): Promise<void> {
    const element = await this.actionable({ enabled: false });
    await act(async () => element.focus());
  }

  async fill(value: string): Promise<void> {
    const element = (await this.actionable({ enabled: true })) as
      HTMLInputElement | HTMLTextAreaElement;
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
    const element = (await this.actionable({ enabled: true })) as HTMLInputElement;
    if (!element.checked) await this.click();
  }

  async select(value: string): Promise<void> {
    const element = (await this.actionable({ enabled: true })) as HTMLSelectElement;
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
    getByRole: (role: string, options: RoleQuery = {}) =>
      new Locator(() => {
        const host = scope();
        return host ? byRole(host, role, options) : [];
      }),
    getByText: (text: TextQuery, options: { exact?: boolean } = {}) =>
      new Locator(() => {
        const host = scope();
        return host ? byText(host, text, options) : [];
      }),
    getByLabel: (text: TextQuery, options: { exact?: boolean } = {}) =>
      new Locator(() => {
        const host = scope();
        return host ? byLabel(host, text, options) : [];
      }),
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

const EXPECT_TIMEOUT_MS = 5_000;

type Check = (locator: Locator) => { pass: boolean; actual: string };

function textOf(locator: Locator): string {
  return locator.text();
}

function accessibleDescription(element: Element): string {
  const ids = element.getAttribute("aria-describedby")?.split(/\s+/) ?? [];
  return collapse(ids.map((id) => document.getElementById(id)?.textContent ?? "").join(" "));
}

function single(locator: Locator, read: (element: HTMLElement) => string | boolean) {
  const found = locator.all();
  if (found.length !== 1) return { ok: false, actual: `${found.length} elements` };
  return { ok: true, value: read(found[0] as HTMLElement) };
}

function onOne(read: (element: HTMLElement) => boolean, label: string): Check {
  return (locator) => {
    const result = single(locator, read);
    return result.ok
      ? { pass: result.value === true, actual: `${label} is ${String(result.value)}` }
      : { pass: false, actual: result.actual ?? "" };
  };
}

function valueOf(
  read: (element: HTMLElement) => string,
  expected: TextQuery,
  exact: boolean,
): Check {
  return (locator) => {
    const result = single(locator, read);
    if (!result.ok) return { pass: false, actual: result.actual ?? "" };
    const actual = String(result.value);
    return { pass: textMatches(actual, expected, exact), actual: JSON.stringify(actual) };
  };
}

const LOCATOR_MATCHERS: Record<string, (...args: never[]) => Check> = {
  toHaveCount: (n: number) => (locator) => ({
    pass: locator.count() === n,
    actual: `${locator.count()} elements`,
  }),
  toBeVisible: () => (locator) => ({
    pass: locator.count() > 0,
    actual: `${locator.count()} visible elements`,
  }),
  toBeHidden: () => (locator) => ({
    pass: locator.count() === 0,
    actual: `${locator.count()} visible elements`,
  }),
  toContainText: (text: TextQuery) => (locator) => ({
    pass: textMatches(textOf(locator), text, false),
    actual: JSON.stringify(textOf(locator)),
  }),
  toHaveText: (text: TextQuery) => (locator) => ({
    pass: textMatches(textOf(locator), text, true),
    actual: JSON.stringify(textOf(locator)),
  }),
  toBeFocused: () => onOne((element) => element === document.activeElement, "focused"),
  toBeDisabled: () => onOne(isDisabledElement, "disabled"),
  toBeEnabled: () => onOne((element) => !isDisabledElement(element), "enabled"),
  toBeChecked: () => onOne((element) => (element as HTMLInputElement).checked, "checked"),
  toBeEditable: () =>
    onOne(
      (element) => !isDisabledElement(element) && !element.hasAttribute("readonly"),
      "editable",
    ),
  toHaveValue: (value: TextQuery) =>
    valueOf((element) => (element as HTMLInputElement).value, value, true),
  toHaveAccessibleDescription: (text: TextQuery) =>
    valueOf(accessibleDescription, text, typeof text === "string"),
  toHaveClass: (name: TextQuery) => valueOf((element) => element.className, name, false),
  toHaveAttribute: (name: string, value?: TextQuery) => (locator) => {
    const result = single(locator, (element) => element.getAttribute(name) ?? "");
    if (!result.ok) return { pass: false, actual: result.actual ?? "" };
    const present = locator.all()[0].hasAttribute(name);
    const actual = String(result.value);
    return {
      pass: value === undefined ? present : present && textMatches(actual, value, true),
      actual: present ? `${name}=${JSON.stringify(actual)}` : `no ${name}`,
    };
  },
};

expect.extend(
  Object.fromEntries(
    Object.entries(LOCATOR_MATCHERS).map(([name, build]) => [
      name,
      async function (this: { isNot: boolean }, received: unknown, ...args: unknown[]) {
        if (!(received instanceof Locator)) throw new TypeError(`${name} expects a Locator`);
        const negated = this.isNot === true;
        const check = (build as (...expected: unknown[]) => Check)(...args);
        let last = check(received);
        const settled = () => {
          last = check(received);
          return last.pass !== negated;
        };
        await pollUntil(settled, () => last.actual, EXPECT_TIMEOUT_MS).catch(() => undefined);
        return {
          pass: last.pass,
          message: () =>
            `expected locator ${negated ? "not " : ""}${name}(${args.map((a) => JSON.stringify(a)).join(", ")}), got ${last.actual}`,
        };
      },
    ]),
  ),
);

declare module "vitest" {
  interface Matchers<R extends void | Promise<void> = void | Promise<void>, T = unknown> {
    toHaveCount(n: number): Promise<void>;
    toBeVisible(): Promise<void>;
    toBeHidden(): Promise<void>;
    toContainText(text: TextQuery): Promise<void>;
    toHaveText(text: TextQuery): Promise<void>;
    toBeFocused(): Promise<void>;
    toBeDisabled(): Promise<void>;
    toBeEnabled(): Promise<void>;
    toBeChecked(): Promise<void>;
    toBeEditable(): Promise<void>;
    toHaveValue(value: TextQuery): Promise<void>;
    toHaveAccessibleDescription(text: TextQuery): Promise<void>;
    toHaveClass(name: TextQuery): Promise<void>;
    toHaveAttribute(name: string, value?: TextQuery): Promise<void>;
  }
}

export { expect };
