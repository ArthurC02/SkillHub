import type { ReactNode } from "react";

function safeHref(value: string): string | undefined {
  try {
    return new URL(value).protocol === "https:" ? value : undefined;
  } catch {
    return undefined;
  }
}

export function ExternalLink({ href, children }: { href: string; children: ReactNode }) {
  const safe = safeHref(href);
  return safe ? <a href={safe}>{children}</a> : <>{children}</>;
}
