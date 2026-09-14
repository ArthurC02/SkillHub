export type IconState = "fail" | "unknown" | "degraded" | "pass";

const PATH: Record<IconState, string> = {
  fail: "M3 3l6 6M9 3l-6 6",
  unknown: "M4.2 4.5a1.8 1.8 0 1 1 2.6 1.6c-.6.3-.8.6-.8 1.2M6 9v.3",
  degraded: "M3.5 6h5",
  pass: "M3 6.2l2 2 4-4.4",
};

export function StateIcon({ state }: { state: IconState }) {
  return (
    <svg aria-hidden="true" width="12" height="12" viewBox="0 0 12 12" focusable="false">
      <path
        d={PATH[state]}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
      />
    </svg>
  );
}
