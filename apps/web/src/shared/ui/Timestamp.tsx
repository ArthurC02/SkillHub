import { useState } from "react";
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

export function Timestamp({ at, relative = false }: { at: string; relative?: boolean }) {
  const [now] = useState(() => Date.now());
  const date = new Date(at);
  if (Number.isNaN(date.getTime())) {
    return <time dateTime={at}>{formatAt(at)}</time>;
  }

  return (
    <time dateTime={at}>
      {formatAt(at)}
      {relative && `（${ago(date, now)}）`}
    </time>
  );
}
