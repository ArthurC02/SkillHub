import { useState } from "react";
import type { SearchFilters } from "../../../../core/api/types";
import { FilterControls } from "./FilterControls";
import { UNAVAILABLE_FILTERS } from "./FilterControls.model";
import "./FilterBar.css";

export function FilterBar({
  filters,
  onChange,
}: {
  filters: SearchFilters;
  onChange: (next: SearchFilters) => void;
}) {
  const narrowing = Object.values(filters).some(Boolean);
  const [open, setOpen] = useState(narrowing);
  const signature = JSON.stringify([
    narrowing,
    filters.script,
    filters.validation,
    filters.agent,
    filters.tier,
    filters.category,
  ]);
  const [trackedSignature, setTrackedSignature] = useState(signature);
  if (signature !== trackedSignature) {
    setTrackedSignature(signature);
    if (narrowing) setOpen(true);
  }

  return (
    <details
      className="filter-disclosure"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>
        篩選條件（{LIVE_FILTERS} 項可用、{UNAVAILABLE_FILTERS.length} 項目前無法篩選）
      </summary>
      <FilterControls filters={filters} onChange={onChange} />
    </details>
  );
}

const LIVE_FILTERS = 5;
