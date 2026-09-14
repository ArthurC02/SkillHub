import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { TraceEvent } from "../../trace.service";

export function TraceEventRow({ event }: { event: TraceEvent }) {
  return (
    <li>
      <p>
        <code>#{event.seq}</code> <Timestamp at={event.occurred_at} /> · {event.emitted_by} ·{" "}
        {event.type}
        {event.status ? ` · ${event.status}` : null}
        {event.late ? " · 遲到" : null}
        {(event.masked_fields?.length ?? 0) > 0
          ? ` · 已遮罩 ${event.masked_fields.length} 個欄位`
          : null}
      </p>
      {/* payload is untrusted sandbox output; stringify + <pre> renders it as inert text, never HTML */}
      <pre>{JSON.stringify(event.payload, null, 2)}</pre>
    </li>
  );
}
