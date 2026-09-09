import { Fragment, type ReactNode } from "react";

/**
 * Makes the characters a person cannot see visible, where a person is about to
 * judge the text (`04` 丙-210).
 *
 * # Why this shows them instead of removing them
 *
 * Go strips the same characters on the way to a model
 * (`llmclient/hidden.go`), and the opposite is right here, for two reasons.
 *
 * A Skill body is what somebody approves, and iron rule 4 makes a Skill
 * Version immutable — quietly rewriting it before display is asking a person
 * to sign something they were not shown. And the attack aimed at the person is
 * a different one from the attack aimed at the model: Trojan Source
 * (CVE-2021-42574) uses a bidirectional override to make a body read one way
 * on screen and store another. Removing the override would hide the deception
 * rather than the payload. Bitbucket and Red Hat's supply-chain checks reached
 * the same conclusion for source review: highlight, do not rewrite.
 *
 * # What counts
 *
 * The same rule Go uses, so the two halves cannot drift into disagreeing about
 * what "invisible" means: Unicode category `Cf`, minus ZWNJ and ZWJ, which are
 * orthography rather than smuggling. Written as escapes because a literal
 * would be a character this file's own reader cannot see.
 */
const FORMAT = /\p{Cf}/u;

function hidden(c: string): boolean {
  return c !== "\u200C" && c !== "\u200D" && FORMAT.test(c);
}

/** U+E0041 → "U+E0041", so the marker names what it found. */
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
