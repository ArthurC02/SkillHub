import type { Dispatch, SetStateAction } from "react";
import type { CreationSession, CreationSnapshot } from "../../creation.service";
import type { RunListItem } from "../../../runs";
import { buildRoundTimeline } from "../create.model";
import type { Perform } from "../create.commands";
import { ConversationLog } from "./ConversationLog";
import {
  BriefCard,
  DiagramDescriptionCard,
  DiagramInterpretationCard,
  DiagramUnderstandingCard,
  DuplicatesCard,
  FetchConsentCard,
  FetchedPages,
  ReferencesCard,
  RoundTimeline,
} from "./SessionCards";
import { DraftCard } from "./DraftCard";

type DiagramAnswers = Record<string, string>;

export function SessionFeed({
  session,
  thumbs,
  working,
  busy,
  terminal,
  locked,
  latest,
  perform,
  diagramAnswers,
  onDiagramAnswers,
}: {
  session: CreationSession;
  thumbs: Map<string, string>;
  working: boolean;
  busy: boolean;
  terminal: boolean;
  locked: boolean;
  latest: RunListItem | null | undefined;
  perform: Perform;
  diagramAnswers: DiagramAnswers;
  onDiagramAnswers: Dispatch<SetStateAction<DiagramAnswers>>;
}) {
  const p = session.snapshot;
  const roundTimeline = buildRoundTimeline(p.messages);
  return (
    <>
      <ConversationLog
        session={session}
        thumbs={thumbs}
        working={working}
        busy={busy}
        perform={perform}
      />
      {roundTimeline.length > 0 && <RoundTimeline items={roundTimeline} />}
      {p.brief && <BriefCard p={p} locked={locked} perform={perform} />}
      {session.state === "needs_reupload" && (
        <p className="system-line">這一步中斷了，Agent 沒能讀出那張圖；請在下面重新上傳同一張。</p>
      )}
      <DiagramCards
        p={p}
        diagramAnswers={diagramAnswers}
        onDiagramAnswers={onDiagramAnswers}
        locked={locked}
        perform={perform}
      />
      {p.pending_action === "confirm_fetch" && p.pending_fetch_url && (
        <FetchConsentCard url={p.pending_fetch_url} locked={locked} perform={perform} />
      )}
      {!!p.fetches?.length && <FetchedPages fetches={p.fetches} />}
      {(p.references.length > 0 || p.pending_action === "confirm_references") && (
        <ReferencesCard
          references={p.references}
          pendingAction={p.pending_action}
          catalogChecked={p.catalog_checked}
          locked={locked}
          perform={perform}
        />
      )}
      {p.pending_action === "confirm_duplicate" && p.duplicates && p.duplicates.length > 0 && (
        <DuplicatesCard
          duplicates={p.duplicates}
          contentHash={p.draft?.content_hash}
          locked={locked}
          perform={perform}
        />
      )}
      {p.draft && (
        <DraftCard
          p={p}
          draft={p.draft}
          state={session.state}
          terminal={terminal}
          latest={latest}
          locked={locked}
          perform={perform}
        />
      )}
    </>
  );
}

function DiagramCards({
  p,
  diagramAnswers,
  onDiagramAnswers,
  locked,
  perform,
}: {
  p: CreationSnapshot;
  diagramAnswers: DiagramAnswers;
  onDiagramAnswers: Dispatch<SetStateAction<DiagramAnswers>>;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <>
      {p.diagram_understanding && (
        <DiagramUnderstandingCard
          understanding={p.diagram_understanding}
          confirmed={p.diagram_confirmed}
          pendingAction={p.pending_action}
          locked={locked}
          perform={perform}
        />
      )}
      {p.diagram_description && (
        <DiagramDescriptionCard
          description={p.diagram_description}
          confirmed={p.diagram_description_confirmed}
          pendingAction={p.pending_action}
          locked={locked}
          perform={perform}
        />
      )}
      {p.diagram_interpretation && (
        <DiagramInterpretationCard
          interpretation={p.diagram_interpretation}
          confirmed={p.diagram_confirmed}
          pendingAction={p.pending_action}
          answers={diagramAnswers}
          onAnswer={(uncertaintyID, answer) =>
            onDiagramAnswers((old) => ({ ...old, [uncertaintyID]: answer }))
          }
          locked={locked}
          perform={perform}
        />
      )}
    </>
  );
}
