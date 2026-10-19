ALTER TABLE publications ADD COLUMN retired_at timestamptz;

ALTER TABLE publications DROP CONSTRAINT publications_publisher_id_name_key;

CREATE UNIQUE INDEX publications_live_name_key ON publications (publisher_id, name) WHERE retired_at IS NULL;

CREATE INDEX publications_retired_name_idx ON publications (publisher_id, name, retired_at DESC) WHERE retired_at IS NOT NULL;
