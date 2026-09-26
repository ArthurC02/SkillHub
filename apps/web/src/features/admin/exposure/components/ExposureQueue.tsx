import { Link } from "@tanstack/react-router";
import type { ExposureQueueEntry } from "../../admin.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";

export function ExposureQueue({ entries }: { entries: ExposureQueueEntry[] }) {
  if (entries.length === 0) return <p>沒有等待審核的發佈物：0 筆。</p>;
  return (
    <ul className="download-list">
      {entries.map((entry) => (
        <li className="download-item" key={entry.address}>
          <p>
            <strong>
              {entry.publisher}/{entry.name}
            </strong>
          </p>
          <p className="note">
            版本 {entry.release.version_number}，發佈於 <Timestamp at={entry.release.released_at} />
          </p>
          {entry.reviewed_again && <p className="note">曾核准，之後內容有變，需要重新審核。</p>}
          <p>
            <Link to="/admin/exposure" search={{ publication: `${entry.publisher}/${entry.name}` }}>
              審這一筆
            </Link>
          </p>
        </li>
      ))}
    </ul>
  );
}
