import type { ReactElement } from "react";
import { facetNoteLines, type FacetNote } from "./FacetNotes.model";

export function FacetNotes<Row>({
  rows,
  facets,
}: {
  rows: Row[];
  facets: Array<FacetNote<Row>>;
}): ReactElement {
  return (
    <>
      {facetNoteLines(rows, facets).map(({ text }) => (
        <p key={text} className="note">
          {text}
        </p>
      ))}
    </>
  );
}
