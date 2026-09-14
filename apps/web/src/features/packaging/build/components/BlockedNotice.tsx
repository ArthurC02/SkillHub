import type { PackagingBlockedReason } from "../../packaging.service";
import { PACKAGING_BLOCKED_LABEL } from "../../packaging.model";

export function BlockedNotice({
  reason,
  message,
}: {
  reason: PackagingBlockedReason;
  message?: string;
}) {
  return (
    <div className="notice notice-danger" role="status">
      <p>
        <strong>不能打包</strong>：{PACKAGING_BLOCKED_LABEL[reason]}
      </p>
      {message && <p className="note">平台的說法：{message}</p>}
      <p className="note">
        原因代碼 <code>{reason}</code>。
      </p>
    </div>
  );
}
