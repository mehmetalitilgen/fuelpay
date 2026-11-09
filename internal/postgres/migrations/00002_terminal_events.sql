-- +goose Up
-- Append-only audit trail of terminal lifecycle changes. A row is written in
-- the same transaction as the change it describes.
CREATE TABLE terminal_events (
    id          uuid        PRIMARY KEY,
    terminal_id uuid        NOT NULL REFERENCES terminals (id),
    type        text        NOT NULL,
    from_status text,
    to_status   text        NOT NULL,
    request_id  text        NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL,
    CONSTRAINT terminal_events_type_check CHECK (type IN ('registered', 'status_changed'))
);

CREATE INDEX terminal_events_terminal_id_occurred_at_idx ON terminal_events (terminal_id, occurred_at, id);

-- +goose Down
DROP TABLE terminal_events;
