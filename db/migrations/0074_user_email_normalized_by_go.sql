UPDATE users SET email = lower(btrim(email)) WHERE email <> lower(btrim(email));

CREATE INDEX users_email_idx ON users (email);
