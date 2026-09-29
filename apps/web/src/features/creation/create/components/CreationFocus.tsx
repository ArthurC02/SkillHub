import type { CreationSession } from "../../creation.service";
import { creationFocus, creationJourney } from "../create.model";

const JOURNEY_STATUS = {
  complete: "已完成",
  current: "目前",
  upcoming: "稍後",
} as const;

export function CreationFocusPanel({ session }: { session: CreationSession | undefined }) {
  if (!session) return null;
  const focus = creationFocus(session);
  const journey = creationJourney(session);
  return (
    <section className="creation-workbench" aria-labelledby="creation-journey-title">
      <div className="creation-journey-heading">
        <div>
          <span className="creation-eyebrow">Studio 工作進度</span>
          <h2 id="creation-journey-title">從想法走到可驗證版本</h2>
        </div>
        <span className="creation-journey-count">
          {journey.filter((item) => item.status === "complete").length} / {journey.length} 完成
        </span>
      </div>
      <ol className="creation-journey">
        {journey.map((item, index) => (
          <li
            key={item.id}
            data-status={item.status}
            aria-current={item.status === "current" ? "step" : undefined}
          >
            <span className="creation-journey-index" aria-hidden="true">
              {item.status === "complete" ? "✓" : index + 1}
            </span>
            <span>
              <strong>{item.title}</strong>
              <small>{item.description}</small>
              <span className="creation-journey-status">{JOURNEY_STATUS[item.status]}</span>
            </span>
          </li>
        ))}
      </ol>
      {focus && (
        <div className="creation-focus">
          <div>
            <span className="creation-eyebrow">目前待決定</span>
            <h3>{focus.title}</h3>
            <p>{focus.description}</p>
          </div>
          <a className="creation-focus-link" href={`#${focus.target}`}>
            前往這一步
          </a>
        </div>
      )}
    </section>
  );
}
