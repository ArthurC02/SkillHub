import type { PackagingPreview } from "../../packaging.service";
import { BlockedNotice } from "./BlockedNotice";
import { Dependencies } from "./Dependencies";
import { Findings } from "./Findings";

export function PreviewReport({ preview }: { preview: PackagingPreview }) {
  return (
    <>
      {preview.allowed ? (
        <p>這些設定可以打包。</p>
      ) : preview.blocked_reason ? (
        <BlockedNotice reason={preview.blocked_reason} message={preview.blocked_message} />
      ) : (
        <p role="alert">伺服器說不能打包，但沒有給原因代碼。</p>
      )}

      <Findings validation={preview.validation} />
      <Dependencies preview={preview} />

      <h3>會一起打包的 Test Case</h3>
      {preview.included_test_cases.length === 0 ? (
        <p className="note">沒有 Test Case 會進包。這不代表這個 Skill 沒有 Test Case。</p>
      ) : (
        <ul className="risk-list">
          {preview.included_test_cases.map((tc) => (
            <li key={tc.test_case_id}>
              {tc.name} <code>test-cases/{tc.slug}/</code>
            </li>
          ))}
        </ul>
      )}

      <h3>打包器拿掉的檔案</h3>
      {preview.excluded_files.length === 0 ? (
        <p className="note">沒有檔案被排除，這一份帶走的就是版本裡的全部內容。</p>
      ) : (
        <ul className="risk-list">
          {preview.excluded_files.map((f) => (
            <li key={f.path}>
              <code>{f.path}</code> {f.label}
              <span className="note">：{f.note}</span>
            </li>
          ))}
        </ul>
      )}

      <h3>不會進包的 Test Case</h3>
      {preview.excluded_test_cases.length === 0 ? (
        <p className="note">沒有被排除的項目。</p>
      ) : (
        <ul className="risk-list">
          {preview.excluded_test_cases.map((tc) => (
            <li key={tc.test_case_id}>
              {tc.name} {tc.label}
              <span className="note">：{tc.note}</span>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
