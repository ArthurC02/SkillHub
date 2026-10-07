import type { PlatformAgentRun } from "../../admin.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import "./DailyReport.css";

const DAILY_REPORT_AGENT = "daily-report";

type ReportItem = { status: "fine" | "attention"; text: string; cites: string[] };

function reportItems(run: PlatformAgentRun): ReportItem[] {
  const items = run.result?.items;
  return Array.isArray(items) ? (items as ReportItem[]) : [];
}

export function DailyReport({ run }: { run: PlatformAgentRun }) {
  if (run.agent !== DAILY_REPORT_AGENT) return null;
  const items = reportItems(run);
  if (items.length === 0) return <p>這次執行沒有交出日報。</p>;
  const attention = items.filter((item) => item.status === "attention");
  const fine = items.filter((item) => item.status === "fine");
  return (
    <>
      <p className="note">
        <Timestamp at={run.finished_at ?? run.started_at} />{" "}
        {run.status === "completed"
          ? "完成。每一項都附它根據的事實，平台已核對這些事實都在當天的維運報表裡。"
          : "這份日報沒有通過核對，所以沒有進待辦；原因寫在執行紀錄上。"}
      </p>
      {[
        { heading: `需要注意：${attention.length} 項`, list: attention },
        { heading: `正常：${fine.length} 項`, list: fine },
      ].map(({ heading, list }) => (
        <section key={heading}>
          <h3>{heading}</h3>
          <ul className="download-list">
            {list.map((item) => (
              <li className="download-item" key={item.text}>
                <p>{item.text}</p>
                <p className="note">
                  依據：
                  {item.cites.map((cite) => (
                    <code className="daily-report-cite" key={cite}>
                      {cite}
                    </code>
                  ))}
                </p>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}
