export function focusAndCenter(element: HTMLElement | null | undefined) {
  element?.focus();
  element?.scrollIntoView?.({ block: "center" });
}
