export function IncompleteNotice({ complete }: { complete: boolean }) {
  if (complete) return null;
  return (
    <p role="status" className="notice">
      部分事件未送達，以下內容可能不完整。
    </p>
  );
}
