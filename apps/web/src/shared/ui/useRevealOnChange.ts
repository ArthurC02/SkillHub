import { useLayoutEffect, useRef } from "react";

export function useRevealOnChange<T extends HTMLElement>(message: string | null | undefined) {
  const ref = useRef<T>(null);
  useLayoutEffect(() => {
    if (message) ref.current?.scrollIntoView?.({ block: "nearest" });
  }, [message]);
  return ref;
}
