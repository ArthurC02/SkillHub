import { act } from "react";
import type { App, Locator } from "./app";

export async function chooseZipWithoutNativeValidation(page: App, name: string, content: string) {
  const input = page.getByLabel("小工具 zip", { exact: true }).element() as HTMLInputElement;
  Object.defineProperty(input, "files", {
    configurable: true,
    value: [new File([content], name, { type: "application/zip" })],
  });
  input.removeAttribute("required");
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

export function chooseCsv(input: Locator, name: string, content: string) {
  const element = input.element();
  const file = new File([content], name, { type: "text/csv" });
  Object.defineProperty(element, "files", { value: [file], configurable: true });
  Object.defineProperty(element, "value", {
    value: `C:\\fakepath\\${name}`,
    writable: true,
    configurable: true,
  });
}
