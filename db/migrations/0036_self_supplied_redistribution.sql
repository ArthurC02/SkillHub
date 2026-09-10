ALTER TABLE skills
    DROP CONSTRAINT IF EXISTS skills_redistribution_check;

ALTER TABLE skills
    ADD CONSTRAINT skills_redistribution_check
        CHECK (redistribution IN ('allowed', 'blocked', 'unknown', 'self_supplied'));

COMMENT ON COLUMN skills.redistribution IS
    'May a Download Artifact be produced from this skill? ''allowed'' (a verdict about the licence) and ''self_supplied'' (this workspace brought the bytes, so handing them back is not redistribution) release; ''unknown'' and ''blocked'' refuse. license_status = Confirmed must never set this on its own (CONTENT-002). Copied onto forks at fork time, like access_restriction. See 0027 and 0036.';

