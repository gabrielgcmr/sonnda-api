-- supabase/migrations/20261009132454_create_capture_sessions_and_captures.sql
CREATE TABLE public.capture_sessions (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
    pairing_code_hash bytea NOT NULL UNIQUE,
    pairing_expires_at timestamptz NOT NULL,
    upload_token_hash bytea UNIQUE,
    upload_token_expires_at timestamptz,
    claimed_at timestamptz,
    desktop_last_seen_at timestamptz NOT NULL,
    mobile_last_seen_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT capture_sessions_id_account_key UNIQUE (id, account_id),
    CONSTRAINT capture_sessions_pairing_hash_length CHECK (octet_length(pairing_code_hash) = 32),
    CONSTRAINT capture_sessions_upload_hash_length CHECK (
        upload_token_hash IS NULL OR octet_length(upload_token_hash) = 32
    ),
    CONSTRAINT capture_sessions_pairing_expiration CHECK (
        pairing_expires_at > created_at
        AND pairing_expires_at <= created_at + interval '5 minutes'
    ),
    CONSTRAINT capture_sessions_claim_consistency CHECK (
        (claimed_at IS NULL AND upload_token_hash IS NULL AND upload_token_expires_at IS NULL)
        OR
        (claimed_at IS NOT NULL AND upload_token_hash IS NOT NULL
            AND upload_token_expires_at IS NOT NULL
            AND claimed_at >= created_at
            AND upload_token_expires_at > claimed_at
            AND upload_token_expires_at <= claimed_at + interval '12 hours')
    ),
    CONSTRAINT capture_sessions_presence_consistency CHECK (
        desktop_last_seen_at >= created_at
        AND (
            mobile_last_seen_at IS NULL
            OR (claimed_at IS NOT NULL AND mobile_last_seen_at >= claimed_at)
        )
    ),
    CONSTRAINT capture_sessions_revocation_consistency CHECK (
        revoked_at IS NULL OR revoked_at >= created_at
    ),
    CONSTRAINT capture_sessions_updated_at_consistency CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX capture_sessions_one_unrevoked_per_account_idx
    ON public.capture_sessions(account_id)
    WHERE revoked_at IS NULL;
CREATE INDEX capture_sessions_account_created_at_idx
    ON public.capture_sessions(account_id, created_at DESC);
CREATE INDEX capture_sessions_cleanup_idx
    ON public.capture_sessions(pairing_expires_at, upload_token_expires_at, revoked_at);

CREATE TABLE public.captures (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
    capture_session_id uuid NOT NULL,
    storage_uri text,
    original_filename text NOT NULL,
    mime_type text NOT NULL,
    size_bytes bigint NOT NULL,
    status text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT captures_session_account_fk
        FOREIGN KEY (capture_session_id, account_id)
        REFERENCES public.capture_sessions(id, account_id) ON DELETE RESTRICT,
    CONSTRAINT captures_filename_not_blank CHECK (btrim(original_filename) <> ''),
    CONSTRAINT captures_mime_type CHECK (
        mime_type IN ('application/pdf', 'image/jpeg', 'image/png')
    ),
    CONSTRAINT captures_size CHECK (size_bytes > 0 AND size_bytes <= 10485760),
    CONSTRAINT captures_status CHECK (status IN ('uploading', 'available', 'deleting')),
    CONSTRAINT captures_storage_consistency CHECK (
        status <> 'available' OR (storage_uri IS NOT NULL AND btrim(storage_uri) <> '')
    ),
    CONSTRAINT captures_expiration CHECK (
        expires_at = created_at + interval '24 hours'
    ),
    CONSTRAINT captures_updated_at_consistency CHECK (updated_at >= created_at)
);

CREATE INDEX captures_account_created_at_idx
    ON public.captures(account_id, created_at DESC);
CREATE INDEX captures_session_idx
    ON public.captures(capture_session_id);
CREATE INDEX captures_cleanup_idx
    ON public.captures(status, expires_at, updated_at);

ALTER TABLE public.capture_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.captures ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.capture_sessions, public.captures
    FROM PUBLIC, anon, authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.capture_sessions, public.captures
    TO service_role;
