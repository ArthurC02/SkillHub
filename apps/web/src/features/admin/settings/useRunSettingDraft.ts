import { useState } from "react";

export function useRunSettingDraft(source: number, completed: boolean) {
  const [draft, setDraft] = useState({ source, value: String(source), changed: false });
  if (draft.source !== source || (completed && draft.value !== String(source))) {
    setDraft({
      source,
      value: String(source),
      changed: !completed && draft.value !== String(source),
    });
  }
  return {
    value: draft.value,
    changed: draft.changed && !completed,
    update: (value: string) => setDraft({ source, value, changed: false }),
  };
}
