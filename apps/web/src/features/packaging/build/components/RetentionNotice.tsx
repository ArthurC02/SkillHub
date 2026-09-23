import type { PackagingPreview } from "../../packaging.service";

export function RetentionNotice({ preview }: { preview: PackagingPreview }) {
  const days = preview.retention_days;
  if (typeof days !== "number" || !Number.isFinite(days) || days < 0) {
    return (
      <p className="note" role="status" data-role="evidence">
        這個部署沒有回答打包產物會保留多久，所以這裡不寫數字——
        寫一個沒有人裁定過的期限，比不寫更糟。
      </p>
    );
  }
  return (
    <p className="note" role="status">
      <strong>保留期限</strong>：打包完成後，這份下載套件會保留{" "}
      <strong>{days >= 1 ? `${days} 天` : "不到 1 天"}</strong>，到期後平台自動刪除它。
      <span data-role="teaching">
        <strong>過期不等於做白工</strong>——打包是冪等的，再打一次得到的是同一份內容。
      </span>
    </p>
  );
}
