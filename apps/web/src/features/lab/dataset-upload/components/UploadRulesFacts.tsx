import { Link } from "@tanstack/react-router";
import type { DatasetLimits } from "../../lab.service";
import { roundedBytes, type TestCaseUsage } from "../upload.model";

export function UploadRulesFacts({
  limits,
  used,
  testCase,
}: {
  limits: DatasetLimits;
  used: TestCaseUsage | undefined;
  testCase: string;
}) {
  return (
    <dl data-role="evidence">
      <dt>大小限制</dt>
      <dd>
        單一檔案最大 {roundedBytes(limits.max_file_bytes)};同一個測試題合計最大{" "}
        {roundedBytes(limits.max_test_case_bytes)}、最多 {limits.max_files_per_test_case} 個檔案。
        {used ? (
          <p className="note">
            這個測試題已經用掉 {used.fileCount} 個檔案、
            {roundedBytes(used.totalBytes)}，還可以再上傳{" "}
            {limits.max_files_per_test_case - used.fileCount} 個檔案、
            {roundedBytes(limits.max_test_case_bytes - used.totalBytes)}。 每個檔案在{" "}
            {testCase === "" ? (
              "測試題頁的「測試資料」那一節"
            ) : (
              <Link to="/lab/test-cases/$testCaseId" params={{ testCaseId: testCase }}>
                這個測試題的「測試資料」那一節
              </Link>
            )}
            可以逐一刪除。
          </p>
        ) : (
          <p className="note">正在讀這個測試題已經用掉多少…</p>
        )}
      </dd>

      <dt>支援格式</dt>
      <dd>
        <ul>
          {limits.allowed_kinds.map((k) => (
            <li key={k}>{k}</li>
          ))}
        </ul>
        <p data-role="teaching">檔案類型以內容判定,不看副檔名。</p>
      </dd>

      <dt>保存政策</dt>
      <dd>上傳後保存 {limits.retention_days} 天,到期自動刪除;你也可以隨時自行刪除。</dd>

      <dt>資料使用範圍</dt>
      <dd>
        只有這個測試題的試跑讀得到,不會提供給其他使用者或其他試跑紀錄。 請不要上傳
        Secrets、憑證或個人資料。
      </dd>
    </dl>
  );
}
