import {
  type DiagramUnderstanding,
  diagramSections,
  parseDiagramUnderstanding,
} from "../create.model";

const diagramLabels: Record<keyof DiagramUnderstanding, string> = {
  nodes: "節點",
  conditions: "條件",
  branches: "分支",
  uncertainties: "不確定處",
};

export function DiagramUnderstandingView({ raw }: { raw: string }) {
  const structured = parseDiagramUnderstanding(raw);
  if (!structured)
    return (
      <>
        <p>{raw}</p>
        <p className="note">請在對話要求重新整理，或重新上傳後確認。</p>
      </>
    );
  return (
    <div>
      {diagramSections.map((key) => (
        <section key={key}>
          <h5>{diagramLabels[key]}</h5>
          {structured[key].length > 0 ? (
            <ul>
              {structured[key].map((item, i) => (
                <li key={i}>{item}</li>
              ))}
            </ul>
          ) : (
            <p>未列出</p>
          )}
        </section>
      ))}
    </div>
  );
}
