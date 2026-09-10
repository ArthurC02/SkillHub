import { Fragment, type ReactNode } from "react";
import { Reveal } from "./Reveal";

// Alternation order matters: `**bold**` must be tried before `*em*`, or the
// regex engine matches the shorter `*` pattern first and never sees the bold run.
const INLINE = /(`[^`\n]+`|\*\*[^*\n]+\*\*|\*[^*\n]+\*)/g;

const FENCE = /^\s*```/;
const BULLET = /^\s*[-*]\s+(.*)$/;
const NUMBER = /^\s*\d+[.)]\s+(.*)$/;

function inline(text: string, keyPrefix: string): ReactNode[] {
  return text.split(INLINE).map((piece, i) => {
    const key = keyPrefix + ":" + i;
    if (piece.startsWith("`") && piece.endsWith("`") && piece.length > 1) {
      return <code key={key}>{piece.slice(1, -1)}</code>;
    }
    if (piece.startsWith("**") && piece.endsWith("**") && piece.length > 3) {
      return <strong key={key}>{piece.slice(2, -2)}</strong>;
    }
    if (piece.startsWith("*") && piece.endsWith("*") && piece.length > 2) {
      return <em key={key}>{piece.slice(1, -1)}</em>;
    }
    return <Fragment key={key}>{piece}</Fragment>;
  });
}

type Block =
  | { kind: "code"; lines: string[] }
  | { kind: "list"; ordered: boolean; items: string[] }
  | { kind: "para"; lines: string[] };

export function blocks(text: string): Block[] {
  const out: Block[] = [];
  let fenced: string[] | null = null;

  for (const line of text.split("\n")) {
    if (FENCE.test(line)) {
      if (fenced === null) {
        fenced = [];
      } else {
        out.push({ kind: "code", lines: fenced });
        fenced = null;
      }
      continue;
    }
    if (fenced !== null) {
      fenced.push(line);
      continue;
    }

    const last = out[out.length - 1];
    if (line.trim() === "") {
      // An empty para only closes whatever block is open; it never starts one,
      // so leading/trailing blank lines don't produce empty paragraphs.
      if (last) out.push({ kind: "para", lines: [] });
      continue;
    }

    const bullet = BULLET.exec(line);
    const numbered = NUMBER.exec(line);
    if (bullet || numbered) {
      const ordered = !bullet;
      const item = (bullet ?? numbered)![1];
      if (last && last.kind === "list" && last.ordered === ordered) {
        last.items.push(item);
      } else {
        out.push({ kind: "list", ordered, items: [item] });
      }
      continue;
    }

    if (last && last.kind === "para" && last.lines.length > 0) {
      last.lines.push(line);
    } else {
      out.push({ kind: "para", lines: [line] });
    }
  }

  // A fence never closed is still rendered as code: streamed text can end
  // mid-block, and showing the partial code as prose would misread it worse.
  if (fenced !== null && fenced.length > 0) out.push({ kind: "code", lines: fenced });

  return out.filter((b) => b.kind !== "para" || b.lines.length > 0);
}

export function ModelMarkdown({ text }: { text: string }) {
  return (
    <div className="creation-md">
      {blocks(text).map((b, i) => {
        if (b.kind === "code") {
          return (
            <pre className="skill-md" key={i}>
              <Reveal text={b.lines.join("\n")} />
            </pre>
          );
        }
        if (b.kind === "list") {
          const items = b.items.map((item, j) => <li key={j}>{inline(item, i + "." + j)}</li>);
          return b.ordered ? <ol key={i}>{items}</ol> : <ul key={i}>{items}</ul>;
        }
        return <p key={i}>{inline(b.lines.join("\n"), String(i))}</p>;
      })}
    </div>
  );
}
