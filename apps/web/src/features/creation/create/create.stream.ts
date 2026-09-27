import { useEffect, useRef, useState } from "react";
import { isBelow, newestMessageIn } from "./create.model";

export function useMessageStream(
  id: string,
  messageCount: number,
  newestRole: string | undefined,
  working: boolean,
) {
  const stream = useRef<HTMLDivElement>(null);
  const [latestHidden, setLatestHidden] = useState(false),
    [unseen, setUnseen] = useState(0);
  useEffect(() => {
    const el = stream.current;
    if (!el) return;
    const onScroll = () => {
      const hidden = isBelow(newestMessageIn(el), el);
      setLatestHidden(hidden);
      if (!hidden) setUnseen(0);
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);
  const showLatest = () => {
    if (stream.current) newestMessageIn(stream.current)?.scrollIntoView?.({ block: "start" });
    setLatestHidden(false);
    setUnseen(0);
  };
  const seen = useRef({ id: "", count: 0 });
  useEffect(() => {
    const before = seen.current;
    seen.current = { id, count: messageCount };
    const reopened = before.id !== id || before.count === 0;
    const added = messageCount - (reopened ? 0 : before.count);
    if (added <= 0) return;
    if (!reopened && latestHidden && newestRole !== "user") setUnseen((n) => n + added);
    else showLatest();
  }, [id, messageCount, newestRole, latestHidden]);
  useEffect(() => {
    if (working)
      stream.current
        ?.querySelector(".creation-log > li[data-pending]")
        ?.scrollIntoView?.({ block: "nearest" });
  }, [working]);
  return { stream, latestHidden, unseen, showLatest };
}
