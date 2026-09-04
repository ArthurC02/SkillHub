-- 0055_generation_inputs: a generated package can now come from more than a
-- sentence (02:GEN-005 diagram, 02:GEN-006 reference skills; ADR-066), and the
-- provenance row has to name every input or ADR-047 決策 1's "this row
-- reproduces the package" stops being true.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- One jsonb column rather than three: the shape is a small closed document
-- written by exactly one Go function and read back by one, never queried by
-- key. What goes in it:
--
--   {"diagram":    {"media_type": "image/png", "sha256": "…", "bytes": 12345},
--    "references": [{"skill_id": "…", "version_id": "…", "name": "…"}]}
--
-- The diagram's BYTES are deliberately not stored anywhere: the image is an
-- input to one synchronous call, and keeping it would put a user-uploaded
-- object under the object-cleanup workers (0039, 0051) with no manifest to
-- protect it. Its digest is enough to tell whether a re-upload is the same
-- picture. Reference skills are recorded by id and version so the row still
-- says what was read after the source is deleted or re-versioned.
--
-- NULL for git and upload rows and for a text-only generation, which the
-- existing three columns already describe in full.
ALTER TABLE skill_sources ADD COLUMN generation_inputs jsonb;

COMMENT ON COLUMN skill_sources.generation_inputs IS
    'Inputs beyond task_description that produced a generated package (ADR-066): the diagram''s digest, media type and byte count, and the reference skills'' ids and names. NULL for git, upload and text-only generations. Never the image bytes.';
