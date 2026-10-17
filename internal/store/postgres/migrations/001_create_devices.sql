CREATE TABLE devices (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL,
    address    TEXT NOT NULL,
    status     TEXT NOT NULL,
    version    BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX devices_kind_idx ON devices (kind);
CREATE INDEX devices_status_idx ON devices (status);
