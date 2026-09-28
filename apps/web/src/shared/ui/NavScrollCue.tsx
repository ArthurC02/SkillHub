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
    update();
    nav.addEventListener("scroll", update, { passive: true });
    const observer = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(update);
    observer?.observe(nav);
    return () => {
      nav.removeEventListener("scroll", update);
      observer?.disconnect();
    };
  }, []);

  return (
    <span ref={cueRef} className="nav-scroll-cue" aria-hidden="true" hidden={hidden}>
      更多 →
    </span>
  );
}
