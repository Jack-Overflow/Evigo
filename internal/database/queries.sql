-- name: ListPending :many
SELECT device_id, attempts, last_attempt_ns, time_left_ns, grace_left_ns FROM pending;

-- name: DeletePending :exec
DELETE FROM pending;

-- name: InsertPending :exec
INSERT INTO pending (device_id, attempts, last_attempt_ns, time_left_ns, grace_left_ns)
VALUES (?, ?, ?, ?, ?);
