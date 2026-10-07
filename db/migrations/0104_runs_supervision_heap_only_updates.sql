DROP INDEX runs_supervision_fairness_idx;

ALTER TABLE runs SET (fillfactor = 85);
