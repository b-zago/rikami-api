INSERT INTO roles (role_name, sleep, awake, regen, update, kill, summon, users)
VALUES
    ('admin', TRUE, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE),
    ('moderator', TRUE, TRUE, TRUE, TRUE, TRUE, TRUE, FALSE),
    ('maintainer',TRUE, TRUE, TRUE, TRUE, FALSE, FALSE, FALSE);
