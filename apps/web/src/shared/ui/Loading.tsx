export function Loading({ what, className }: { what: string; className?: string }) {
  return (
    <p role="status" data-loading="" className={className}>
      載入{what}中…
    </p>
  );
}
