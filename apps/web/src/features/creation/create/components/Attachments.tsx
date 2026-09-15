import type { CreationAttachment } from "../../creation.service";
import "./Attachments.css";

export function Attachments({
  list,
  thumbs,
}: {
  list: CreationAttachment[];
  thumbs: Map<string, string>;
}) {
  return (
    <ul className="creation-attachments">
      {list.map((a) => {
        const url = thumbs.get(a.sha256);
        return (
          <li key={a.sha256} className="creation-attachment">
            {url ? (
              <img src={url} alt={"你在這一輪附上的流程圖（" + a.media_type + "）"} />
            ) : (
              <p className="note">平台不保存原圖，所以重新整理之後這裡只剩它的說明。</p>
            )}
            <span className="note">
              流程圖 · {a.media_type} · {a.bytes} 位元組
            </span>
          </li>
        );
      })}
    </ul>
  );
}
