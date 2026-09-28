import { Link } from "@tanstack/react-router";
import { downloadHref, type CreatedDownloadArtifact } from "../../packaging.service";
import { DownloadArtifactFacts } from "../../components/DownloadArtifactFacts";

export function BuiltResultNotice({
  built,
  onDownload,
}: {
  built: CreatedDownloadArtifact;
  onDownload: () => void;
}) {
  return (
    <div>
      <p role="status">
        {built.duplicate
          ? "已有相同套件：同一個版本、同一個目標、同一個 Test Case 選項先前就打過，這就是那一份，不是第二份。"
          : "套件已建立。"}
      </p>
      <DownloadArtifactFacts artifact={built} />
      <p className="note">
        上面折起來的那兩串是雜湊，不是簽章。
        <strong>MVP 的套件不帶數位簽章，平台也不驗簽</strong>
        （這是明文的「不做」）——它們證明得了「位元組沒有被改過」， 證明不了「這份東西是誰做的」。
      </p>
      <p>
        <a href={downloadHref(built.artifact_id)} onClick={onDownload}>
          下載 {built.file_name}
        </a>
        {" ｜ "}
        <Link to="/workspace/downloads">到下載紀錄</Link>
      </p>
    </div>
  );
}
