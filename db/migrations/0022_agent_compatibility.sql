CREATE TABLE skill_runtime_compatibility (
    skill_version_id uuid NOT NULL REFERENCES skill_versions (id) ON DELETE CASCADE,

    runtime_image    text NOT NULL CHECK (runtime_image <> ''),

    capability       text NOT NULL
        CHECK (capability IN ('activated', 'not_activated', 'unverified')),

    runtime          text NOT NULL
        CHECK (runtime IN ('native', 'transpiled', 'failed', 'unverified')),

    source_run_id    uuid REFERENCES runs (id),
    measured_at      timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (skill_version_id, runtime_image)
);

CREATE INDEX skill_runtime_compatibility_latest_idx
    ON skill_runtime_compatibility (skill_version_id, measured_at DESC);
