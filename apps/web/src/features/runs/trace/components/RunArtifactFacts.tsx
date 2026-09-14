import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { RunArtifact } from "../../runs.service";

export function RunArtifactFacts({ artifact }: { artifact: RunArtifact }) {
  return (
    <>
      <p>
        <strong>{artifact.file_name}</strong> <span className="badge">{artifact.content_type}</span>
        {artifact.purged && (
          <>
            {" "}
            <span className="badge badge-expired">檔案已不存在</span>
          </>
        )}
      </p>
      <p className="note">
        {artifact.size_bytes} bytes｜建立於 <Timestamp at={artifact.created_at} />
        {artifact.expires_at ? (
          <>
            ｜到期時間 <Timestamp at={artifact.expires_at} />
          </>
        ) : (
          "｜保存期限：尚未定值"
        )}
      </p>
      {artifact.purged && (
        <p className="note">
          內容已被清除（到期或儲存端已不存在），這一列保留，因為「曾經產生過這個檔案」仍然是事實。
        </p>
      )}
      <details>
        <summary>內容雜湊</summary>
        <code>{artifact.content_hash}</code>
      </details>
    </>
  );
}
