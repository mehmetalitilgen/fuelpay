-- +goose Up
CREATE TABLE stations (
    id         uuid        PRIMARY KEY,
    code       text        NOT NULL,
    name       text        NOT NULL,
    city       text        NOT NULL,
    address    text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT stations_code_key UNIQUE (code)
);

CREATE TABLE terminals (
    id            uuid        PRIMARY KEY,
    station_id    uuid        NOT NULL REFERENCES stations (id),
    serial_number text        NOT NULL,
    model         text        NOT NULL,
    status        text        NOT NULL,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    activated_at  timestamptz,
    CONSTRAINT terminals_serial_number_key UNIQUE (serial_number),
    CONSTRAINT terminals_status_check
        CHECK (status IN ('pending', 'active', 'suspended', 'decommissioned'))
);

-- Serves "terminals of a station, oldest first" and the foreign key lookup.
CREATE INDEX terminals_station_id_created_at_idx ON terminals (station_id, created_at, id);

-- +goose Down
DROP TABLE terminals;
DROP TABLE stations;
