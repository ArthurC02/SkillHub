import { useEffect, useState } from "react";

export function useDeadlinePassed(deadline: string | undefined) {
  const [passedDeadline, setPassedDeadline] = useState<string>();
  useEffect(() => {
    const at = deadline ? Date.parse(deadline) : Number.NaN;
    if (!Number.isFinite(at)) return;
    const wait = Math.min(Math.max(at - Date.now(), 0), 2 ** 31 - 1);
    const timer = setTimeout(() => setPassedDeadline(deadline), wait);
    return () => clearTimeout(timer);
  }, [deadline]);
  return deadline !== undefined && passedDeadline === deadline;
}
