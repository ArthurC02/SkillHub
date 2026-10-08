ALTER TABLE dispatch_halts
ADD COLUMN generation integer NOT NULL DEFAULT 1 CHECK (generation > 0);
