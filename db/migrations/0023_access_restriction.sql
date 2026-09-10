ALTER TABLE skills
    ADD COLUMN access_restriction text
        CHECK (access_restriction IS NULL OR btrim(access_restriction) <> '');

COMMENT ON COLUMN skills.access_restriction IS
    'Reason code for a licensing hold on the package materials; NULL = none. Set by review, copied onto forks at fork time. See 0023.';
