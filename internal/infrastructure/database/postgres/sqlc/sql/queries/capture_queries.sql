-- internal/infrastructure/database/postgres/sqlc/sql/queries/capture_queries.sql
-- name: CreateCaptureSession :exec
INSERT INTO capture_sessions (
    id, account_id, pairing_code_hash, pairing_expires_at,
    upload_token_hash, upload_token_expires_at, claimed_at,
    desktop_last_seen_at, mobile_last_seen_at, revoked_at,
    created_at, updated_at
) VALUES (
    @id, @account_id, @pairing_code_hash, @pairing_expires_at,
    @upload_token_hash, @upload_token_expires_at, @claimed_at,
    @desktop_last_seen_at, @mobile_last_seen_at, @revoked_at,
    @created_at, @updated_at
);

-- name: GetCaptureSessionByID :one
SELECT * FROM capture_sessions WHERE id = @id;

-- name: GetCurrentCaptureSession :one
SELECT * FROM capture_sessions
WHERE account_id = @account_id AND revoked_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: RevokeCaptureSessionsByAccount :execrows
UPDATE capture_sessions
SET revoked_at = @revoked_at, updated_at = @revoked_at
WHERE account_id = @account_id AND revoked_at IS NULL;

-- name: ClaimCaptureSession :one
UPDATE capture_sessions
SET upload_token_hash = @upload_token_hash,
    upload_token_expires_at = @upload_token_expires_at,
    claimed_at = @claimed_at,
    updated_at = @claimed_at
WHERE pairing_code_hash = @pairing_code_hash
  AND revoked_at IS NULL
  AND claimed_at IS NULL
  AND pairing_expires_at > @claimed_at
  AND desktop_last_seen_at >= @desktop_presence_cutoff
RETURNING *;

-- name: AuthenticateCaptureMobile :one
SELECT * FROM capture_sessions
WHERE upload_token_hash = @upload_token_hash
  AND revoked_at IS NULL
  AND upload_token_expires_at > @now;

-- name: AuthenticateCaptureUpload :one
SELECT * FROM capture_sessions
WHERE upload_token_hash = @upload_token_hash
  AND revoked_at IS NULL
  AND upload_token_expires_at > @now
  AND desktop_last_seen_at >= @desktop_presence_cutoff;

-- name: TouchCaptureSessionDesktop :execrows
UPDATE capture_sessions
SET desktop_last_seen_at = @seen_at, updated_at = @seen_at
WHERE id = @id AND account_id = @account_id AND revoked_at IS NULL;

-- name: TouchCaptureSessionMobile :execrows
UPDATE capture_sessions
SET mobile_last_seen_at = @seen_at, updated_at = @seen_at
WHERE id = @id
  AND upload_token_hash = @upload_token_hash
  AND revoked_at IS NULL
  AND upload_token_expires_at > @seen_at;

-- name: RevokeCaptureSession :execrows
UPDATE capture_sessions
SET revoked_at = @revoked_at, updated_at = @revoked_at
WHERE id = @id AND account_id = @account_id AND revoked_at IS NULL;

-- name: CreateCapture :exec
INSERT INTO captures (
    id, account_id, capture_session_id, storage_uri, original_filename,
    mime_type, size_bytes, status, expires_at, created_at, updated_at
) VALUES (
    @id, @account_id, @capture_session_id, @storage_uri, @original_filename,
    @mime_type, @size_bytes, @status, @expires_at, @created_at, @updated_at
);

-- name: SetCaptureAvailable :one
UPDATE captures
SET storage_uri = @storage_uri, status = 'available', updated_at = @updated_at
WHERE id = @id AND status = 'uploading'
RETURNING *;

-- name: MarkCaptureDeleting :one
UPDATE captures
SET status = 'deleting', updated_at = @updated_at
WHERE id = @id AND status <> 'deleting'
RETURNING *;

-- name: MarkOwnedCaptureDeleting :one
UPDATE captures
SET status = 'deleting', updated_at = @updated_at
WHERE id = @id
  AND account_id = @account_id
  AND status IN ('available', 'deleting')
RETURNING *;

-- name: GetCaptureByAccount :one
SELECT * FROM captures
WHERE id = @id AND account_id = @account_id;

-- name: GetAvailableCaptureByAccount :one
SELECT * FROM captures
WHERE id = @id
  AND account_id = @account_id
  AND status = 'available'
  AND expires_at > @now;

-- name: ListAvailableCaptures :many
SELECT * FROM captures
WHERE account_id = @account_id
  AND status = 'available'
  AND expires_at > @now
ORDER BY created_at DESC, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListCaptureCleanupCandidates :many
SELECT * FROM captures
WHERE expires_at <= @now
   OR status = 'deleting'
   OR (status = 'uploading' AND updated_at <= @uploading_cutoff)
ORDER BY expires_at, created_at
LIMIT @page_limit;

-- name: DeleteCapture :execrows
DELETE FROM captures WHERE id = @id;

-- name: DeleteOwnedCapture :execrows
DELETE FROM captures
WHERE id = @id AND account_id = @account_id AND status = 'deleting';

-- name: DeleteExpiredCaptureSessions :execrows
DELETE FROM capture_sessions session
WHERE session.id IN (
    SELECT candidate.id
    FROM capture_sessions candidate
    WHERE (
        candidate.revoked_at IS NOT NULL
        OR COALESCE(candidate.upload_token_expires_at, candidate.pairing_expires_at) <= @now
    ) AND NOT EXISTS (
        SELECT 1 FROM captures capture WHERE capture.capture_session_id = candidate.id
    )
    LIMIT @page_limit
);
