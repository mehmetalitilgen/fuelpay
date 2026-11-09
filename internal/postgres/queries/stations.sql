-- name: CreateStation :exec
INSERT INTO stations (id, code, name, city, address, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetStation :one
SELECT id, code, name, city, address, created_at
FROM stations
WHERE id = $1;
