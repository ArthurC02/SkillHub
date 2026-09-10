import { Fragment, type ReactNode } from "react";

const FORMAT = /\p{Cf}/u;

function hidden(c: string): boolean {
  // ZWNJ/ZWJ are format characters too, but they're legitimate in normal text
  // (joining scripts, emoji sequences), so they're excluded from flagging.
  return c !== "\u200C" && c !== "\u200D" && FORMAT.test(c);
}

function name(c: string): string {
  return "U+" + c.codePointAt(0)!.toString(16).toUpperCase().padStart(4, "0");
}

export function Reveal({ text }: { text: string }) {
  const chars = [...text];
  if (!chars.some(hidden)) return <>{text}</>;

  const out: ReactNode[] = [];
  let run = "";
  let names: string[] = [];
  const flushText = () => {
    if (run) out.push(<Fragment key={out.length}>{run}</Fragment>);
    run = "";
  };
  const flushMark = () => {
    if (names.length === 0) return;
    out.push(
      <mark className="hidden-char" key={out.length}>
        隱藏字元 {names.join(" ")}
      </mark>,
    );
    names = [];
  };

  for (const c of chars) {
    if (hidden(c)) {
      flushText();
      names.push(name(c));
    } else {
      flushMark();
      run += c;
    }
  }
  flushText();
  flushMark();
  return <>{out}</>;
}
