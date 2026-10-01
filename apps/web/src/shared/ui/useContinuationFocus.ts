import { useEffect, useRef, type RefObject } from "react";

export function useContinuationFocus(
  target: string | undefined,
  available: boolean,
  element: RefObject<HTMLElement | null>,
) {
  const focused = useRef<string | undefined>(undefined);
  const focusMoved = useRef(false);

  useEffect(() => {
    focusMoved.current = false;
    const owner = element.current?.ownerDocument ?? document;
    const onFocus = () => {
      focusMoved.current = true;
    };
    owner.addEventListener("focusin", onFocus);
    return () => owner.removeEventListener("focusin", onFocus);
  }, [element, target]);

  useEffect(() => {
    if (!target) {
      focused.current = undefined;
      return;
    }
    if (!available || focused.current === target || !element.current) return;
    focused.current = target;
    if (!focusMoved.current) element.current.focus();
  }, [available, element, target]);
}
