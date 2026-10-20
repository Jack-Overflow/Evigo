-- name: ListPending :many
SELECT device_id, attempts, last_attempt_ns, time_left_ns FROM pending;

-- name: DeletePending :exec
DELETE FROM pending;

-- name: InsertPending :exec
INSERT INTO pending (device_id, attempts, last_attempt_ns, time_left_ns)
VALUES (?, ?, ?, ?);

-- name: InsertHistory :exec
INSERT INTO history (at_ns, kind, device_id, details)
VALUES (?, ?, ?, ?);

-- name: TrimHistory :exec
DELETE FROM history WHERE history.id <= (SELECT max(newest.id) FROM history AS newest) - sqlc.arg(keep);

-- name: ListLatestHistory :many
SELECT id, at_ns, kind, device_id, details FROM history
ORDER BY id DESC
LIMIT ?;

-- name: ListHistoryBefore :many
SELECT id, at_ns, kind, device_id, details FROM history
WHERE id < sqlc.arg(before_id)
ORDER BY id DESC
LIMIT sqlc.arg(page_size);
