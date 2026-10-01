import { useEffect, useState } from "react";
import { formatAt } from "./Timestamp.model";

const RELATIVE = new Intl.RelativeTimeFormat("zh-TW", { numeric: "always" });

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["second", 60],
  ["minute", 60],
  ["hour", 24],
  ["day", 7],
  ["week", 4.35],
  ["month", 12],
  ["year", Infinity],
];

function ago(from: Date, now: number): string {
  let value = (now - from.getTime()) / 1000;
  // A negative delta means clock skew, not a future event; show it as "just now"
  // rather than a relative phrase like "in -1 seconds".
  if (value < 0) return "剛剛";
  for (const [unit, size] of UNITS) {
    if (value < size) return RELATIVE.format(-Math.floor(value), unit);
    value /= size;
  }
  return RELATIVE.format(-Math.floor(value), "year");
}

function refreshDelay(from: number, now: number): number {
  const elapsed = now - from;
  if (elapsed < 0) return Math.min(-elapsed, 60_000);
  if (elapsed < 60_000) return 1_000 - (elapsed % 1_000);
  if (elapsed < 3_600_000) return 60_000 - (elapsed % 60_000);
  if (elapsed < 86_400_000) return 3_600_000 - (elapsed % 3_600_000);
  return 86_400_000 - (elapsed % 86_400_000);
}

export function Timestamp({ at, relative = false }: { at: string; relative?: boolean }) {
  const [now, setNow] = useState(() => Date.now());
  const date = new Date(at);
  const dateTime = date.getTime();

  useEffect(() => {
    if (!relative || Number.isNaN(dateTime)) return;
    let timer = 0;
    const update = () => {
      const current = Date.now();
      setNow(current);
      timer = window.setTimeout(update, refreshDelay(dateTime, current));
    };
    timer = window.setTimeout(update, 0);
    return () => window.clearTimeout(timer);
  }, [dateTime, relative]);

  if (Number.isNaN(dateTime)) {
    return <time dateTime={at}>{formatAt(at)}</time>;
  }

  return (
    <time dateTime={at}>
      {formatAt(at)}
      {relative && `（${ago(date, now)}）`}
    </time>
  );
}
