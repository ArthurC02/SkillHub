ALTER TABLE workspaces ADD COLUMN is_catalog boolean NOT NULL DEFAULT false;

CREATE INDEX workspaces_is_catalog_idx ON workspaces (id) WHERE is_catalog;
