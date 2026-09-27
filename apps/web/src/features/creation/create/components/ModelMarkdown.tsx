import { Reveal } from "../../../../shared/ui/Reveal";
import { blocks, inline } from "./ModelMarkdown.model";
import "./ModelMarkdown.css";

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
