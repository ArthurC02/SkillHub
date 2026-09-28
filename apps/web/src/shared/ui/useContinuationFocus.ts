import { useEffect, useRef, type RefObject } from "react";

export function useContinuationFocus(
  target: string | undefined,
  available: boolean,
  element: RefObject<HTMLElement | null>,
) {
  const focused = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!target) {
      focused.current = undefined;
      return;
    }
    if (!available || focused.current === target || !element.current) return;
    focused.current = target;
    const active = element.current.ownerDocument.activeElement;
    if (active && active !== element.current.ownerDocument.body && active !== element.current)
      return;
    element.current.focus();
  }, [available, element, target]);
}
