import { useEffect, useRef, useState } from "react";
import "./NavScrollCue.css";

export function NavScrollCue() {
  const cueRef = useRef<HTMLSpanElement>(null);
  const [hidden, setHidden] = useState(true);

  useEffect(() => {
    const cue = cueRef.current;
    const nav = cue?.parentElement;
    if (!nav) return;

    const update = () => {
      setHidden(Math.ceil(nav.scrollLeft + nav.clientWidth) >= nav.scrollWidth);
    };
    const revealCurrent = (preserveFocusedLink = false) => {
      const current =
        (preserveFocusedLink ? nav.querySelector(":focus") : null) ??
        nav.querySelector('[aria-current="page"]');
      if (current && nav.scrollWidth > nav.clientWidth) {
        const bounds = nav.getBoundingClientRect();
        const item = current.getBoundingClientRect();
        const inset = Number.parseFloat(getComputedStyle(nav).scrollPaddingInlineEnd) || 0;
        nav.scrollLeft +=
          item.left < bounds.left
            ? item.left - bounds.left
            : Math.max(0, item.right - bounds.right + inset);
      }
      update();
    };
    revealCurrent();
    const resize = () => revealCurrent(true);
    nav.addEventListener("scroll", update, { passive: true });
    window.addEventListener("resize", resize);
    const observer = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(resize);
    observer?.observe(nav);
    const selection = new MutationObserver(() => revealCurrent());
    selection.observe(nav, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["aria-current"],
    });
    return () => {
      nav.removeEventListener("scroll", update);
      window.removeEventListener("resize", resize);
      observer?.disconnect();
      selection.disconnect();
    };
  }, []);

  return (
    <span ref={cueRef} className="nav-scroll-cue" aria-hidden="true" hidden={hidden}>
      更多 →
    </span>
  );
}
