import { Fragment, type ReactNode } from "react";

/**
 * The Markdown a model message is allowed to carry — [`05`
 * R-70](../../../../docs/plans/05-pending-rulings.md), signed 2026-09-09.
 *
 * # Why this is hand-written and not `react-markdown`
 *
 * The ruling's whole point is which node types can exist, and the safest way
 * to answer that is a renderer that cannot build the forbidden ones. This file
 * emits `<p>`, `<ul>`, `<ol>`, `<li>`, `<pre><code>`, `<code>`, `<strong>` and
 * `<em>`. There is no code path here that produces an `<a>` or an `<img>`,
 * with or without a bug, so "no links, no images" is a property of the
 * program rather than a filter it applies.
 *
 * That matters because the alternative failed in the field. The exfiltration
 * this excludes needs no script: a prompt-injected model emits an image whose
 * URL carries what it just read, and the browser fetches it on render with
 * nobody clicking (AgentFlayer, EchoLeak, the Copilot Chat and Gemini markdown
 * fixes). The one vendor that tried to allow-list URLs instead of refusing to
 * render — OpenAI's `url_safe` — was bypassed through an open redirect on an
 * allow-listed domain. `harden-react-markdown` is that same allow-list shape,
 * so it is not the answer here either; our answer is zero URLs.
 *
 * It is also the smaller dependency story: `apps/web` has no Markdown package
 * at all, and `react-markdown` arrives with unified/remark/rehype/micromark —
 * a tree whose one genuinely dangerous switch (`rehype-raw`) is a plugin
 * somebody can add later without reading this comment.
 *
 * # What is deliberately NOT handled
 *
 * - **Links, autolinks, images** — the ruling excludes them. A literal
 *   `[text](url)` or `![alt](url)` therefore stays visible as text, which is
 *   the honest outcome: the reader sees exactly what the model wrote.
 * - **Raw HTML** — never parsed, so `<b>x</b>` renders as those characters.
 *   React escapes it; nothing here un-escapes it.
 * - **Headings** — `## x` stays literal. The conversation lives inside a page
 *   whose `h3`/`h4`/`h5` are already spoken for.
 * - **`_underscore_` emphasis** — `*` only. This app's messages are full of
 *   `snake_case` identifiers, and treating those as emphasis would corrupt the
 *   thing the person is reading most carefully.
 *
 * # Scope
 *
 * Assistant messages only. The caller enforces that, and the ruling's reason
 * is that `tool` messages carry whole fetched web pages — text an attacker
 * writes directly, without having to get through a model first.
 */

/** Inline runs, longest marker first so `**` never loses to `*`. */
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

/**
 * Line-based on purpose: every block this ruling allows is decided by the
 * start of a line, so a line scanner is the whole grammar. Nothing recurses,
 * which is also why a fenced block inside a list item is not a case — it comes
 * out as its own code block, and the ruling asks for no more.
 */
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
      // A blank line ends whatever was open; it never starts a block of its
      // own, so trailing newlines cannot grow an empty paragraph.
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

  // An unterminated fence is still a code block: the model was cut off, and
  // showing its code as prose would be the wrong half to guess.
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
              {b.lines.join("\n")}
            </pre>
          );
        }
        if (b.kind === "list") {
          const items = b.items.map((item, j) => <li key={j}>{inline(item, i + "." + j)}</li>);
          return b.ordered ? <ol key={i}>{items}</ol> : <ul key={i}>{items}</ul>;
        }
        // The newlines inside a paragraph are content (04 丙-207) and survive
        // through `white-space: pre-wrap`, exactly as they did before this
        // renderer existed.
        return <p key={i}>{inline(b.lines.join("\n"), String(i))}</p>;
      })}
    </div>
  );
}
