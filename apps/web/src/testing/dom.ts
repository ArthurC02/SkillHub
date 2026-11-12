import { afterEach, beforeEach, vi } from "vitest";

export function stubScrollIntoView() {
  const scrollIntoView = HTMLElement.prototype.scrollIntoView;
  beforeEach(() => {
    HTMLElement.prototype.scrollIntoView = vi.fn();
  });
  afterEach(() => {
    if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
    else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
  });
}

export function describedText(element: Element | null): string {
  return (element?.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ");
}
