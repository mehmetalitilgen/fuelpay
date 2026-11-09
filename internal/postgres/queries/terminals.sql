-- name: CreateTerminal :exec
INSERT INTO terminals (id, station_id, serial_number, model, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetTerminal :one
SELECT id, station_id, serial_number, model, status, created_at, updated_at, activated_at
FROM terminals
WHERE id = $1;

-- name: ListTerminalsByStation :many
SELECT id, station_id, serial_number, model, status, created_at, updated_at, activated_at
FROM terminals
WHERE station_id = $1
ORDER BY created_at, id;

-- UpdateTerminalStatus is a compare-and-set: it changes the row only if the
-- status is still the one the caller read. No row means the terminal is gone
-- or its status changed concurrently.
-- name: UpdateTerminalStatus :one
UPDATE terminals
SET status       = sqlc.arg(new_status),
    updated_at   = sqlc.arg(changed_at),
    activated_at = CASE WHEN sqlc.arg(new_status)::text = 'active'
                        THEN sqlc.arg(changed_at)::timestamptz
                        ELSE activated_at END
WHERE id = sqlc.arg(id) AND status = sqlc.arg(old_status)
RETURNING id, station_id, serial_number, model, status, created_at, updated_at, activated_at;
