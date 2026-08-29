/**
 * One timestamp, in one wording, in the element that carries a machine-readable
 * copy of it.
 *
 * Before this, 29 places interpolated the server's ISO 8601 UTC string straight
 * into a Chinese sentence — 「建立於 2026-08-17T00:00:00Z」. Three things were
 * wrong with that and only the first is cosmetic:
 *
 *  1. **It is not the reader's clock.** `Z` is UTC and the reader is not; a
 *     download that expires 「2026-08-17T00:00:00Z」 expires at 08:00 for the
 *     beta cohort, and the page never said which of the two it meant. 設計 §1.1
 *     asks that the evidence be checkable, and a time in somebody else's
 *     timezone with no label is not.
 *  2. **`<time dateTime>` is the element for this** and none of the 29 used it,
 *     so the exact instant was only ever present as prose. Assistive technology
 *     and anything reading the DOM got the same soup a human did.
 *  3. **Four spellings of one fact.** The raw string, `新 Date(...)` nowhere,
 *     a `｜結束於 …` variant, and `InFlight`'s 「多久沒動了」 — 設計 §3 item 14
 *     asks whether one page says one fact the same way twice.
 *
 * `zh-TW`, not the browser's locale: every string this app renders is
 * Traditional Chinese (`index.html`'s `lang`), and a page that mixes a
 * `en-US` date into a Chinese sentence has picked two answers.
 *
 * NO `title`. 設計 §2.4 第 3 項: a qualification that exists only in a tooltip
 * does not exist on a touch device, and `design-system.test.ts` now fails a
 * `title` whose text is nowhere visible. The `dateTime` attribute carries the
 * precise instant for machines; the reader gets the rendered form.
 */

/**
 * `hour12: false` because a 24-hour clock is what the rest of these strings
 * assume, and `2-digit` throughout so a column of them lines up.
 */
const ABSOLUTE = new Intl.DateTimeFormat("zh-TW", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

/**
 * `numeric: "always"`, so a one-unit gap reads 「1 分鐘前」 rather than 「上個
 * 月」. 設計 §2.12 第 3 條 wants a 「多久沒動了」 that distinguishes running from
 * wedged, and an idiom that rounds a quantity away cannot do that.
 */
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

/** 「N 分鐘前」, in the largest unit that still has a whole number in it. */
function ago(from: Date, now: number): string {
  let value = (now - from.getTime()) / 1000;
  // Clock skew, or an event stamped a second into the future by a server whose
  // clock runs ahead: 「-1 秒前」 would be worse than a word.
  if (value < 0) return "剛剛";
  for (const [unit, size] of UNITS) {
    if (value < size) return RELATIVE.format(-Math.floor(value), unit);
    value /= size;
  }
  return RELATIVE.format(-Math.floor(value), "year");
}

/**
 * The same rendering, as a plain string, for the two places that cannot hold an
 * element: an `<option>`'s label (a `<time>` inside one is invalid HTML) and a
 * helper that returns a sentence rather than markup. They lose `dateTime` —
 * that is the cost, and it is why this is the exception and not the interface.
 */
export function formatAt(at: string): string {
  const date = new Date(at);
  if (Number.isNaN(date.getTime())) return `${at}（無法解讀的時間格式）`;
  return ABSOLUTE.format(date);
}

export function Timestamp({
  at,
  /**
   * Also say how long ago. Only the in-flight surfaces want this: elsewhere a
   * relative time is a number that silently stops being true, because nothing
   * re-renders it. `InFlight` polls every three seconds, so its copy is as
   * fresh as the count beside it.
   */
  relative = false,
}: {
  at: string;
  relative?: boolean;
}) {
  const date = new Date(at);
  // An unparseable value is shown verbatim rather than as 「Invalid Date」 or as
  // nothing: 設計 §2.9 — the reader should see that the platform sent something
  // this page could not read, not a blank where a time belongs.
  if (Number.isNaN(date.getTime())) {
    return <time dateTime={at}>{formatAt(at)}</time>;
  }

  return (
    // The server's own string, not a re-serialisation of it: `toISOString()`
    // would add a `.000` this platform never sent, and the attribute's job is
    // to carry the value the API gave, exactly.
    <time dateTime={at}>
      {ABSOLUTE.format(date)}
      {relative && `（${ago(date, Date.now())}）`}
    </time>
  );
}
