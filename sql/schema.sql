CREATE TABLE roles (
    role_id     SERIAL PRIMARY KEY,
    role_name   VARCHAR(50) NOT NULL UNIQUE,
    sleep       BOOLEAN NOT NULL,
    awake       BOOLEAN NOT NULL,
    regen       BOOLEAN NOT NULL,
    update      BOOLEAN NOT NULL,
    kill        BOOLEAN NOT NULL,
    summon      BOOLEAN NOT NULL,
    users       BOOLEAN NOT NULL
);

CREATE TABLE users (
    user_id     SERIAL PRIMARY KEY,
    role_id     INT NOT NULL REFERENCES roles(role_id),
    user_name   VARCHAR(50) NOT NULL UNIQUE,
    pass        VARCHAR(255) NOT NULL,
    hmac_token  VARCHAR(64) NOT NULL
);
