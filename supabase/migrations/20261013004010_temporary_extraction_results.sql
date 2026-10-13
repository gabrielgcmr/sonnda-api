-- supabase/migrations/20261013004010_temporary_extraction_results.sql
INSERT INTO storage.buckets (id, name, public, file_size_limit, allowed_mime_types)
VALUES ('temporary-extractions', 'temporary-extractions', false, 10485760,
        ARRAY['application/pdf', 'image/jpeg', 'image/png'])
ON CONFLICT (id) DO NOTHING;

CREATE TABLE public.temporary_extractions (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('laboratory', 'imaging')),
    result jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
    CONSTRAINT temporary_extractions_id_account_key UNIQUE (id, account_id)
);

CREATE TABLE public.temporary_extraction_pages (
    extraction_id uuid NOT NULL REFERENCES public.temporary_extractions(id) ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    capture_id uuid REFERENCES public.captures(id) ON DELETE CASCADE,
    storage_uri text,
    original_filename text NOT NULL CHECK (btrim(original_filename) <> ''),
    mime_type text NOT NULL CHECK (mime_type IN ('application/pdf', 'image/jpeg', 'image/png')),
    size_bytes bigint NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 10485760),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (extraction_id, position),
    CONSTRAINT temporary_extraction_pages_source CHECK ((capture_id IS NULL) <> (storage_uri IS NULL)),
    CONSTRAINT temporary_extraction_pages_storage_uri CHECK (storage_uri IS NULL OR btrim(storage_uri) <> '')
);

CREATE UNIQUE INDEX temporary_extraction_pages_capture_unique
    ON public.temporary_extraction_pages(capture_id) WHERE capture_id IS NOT NULL;
CREATE INDEX temporary_extractions_account_created_idx
    ON public.temporary_extractions(account_id, created_at DESC);
CREATE INDEX temporary_extractions_expiry_idx
    ON public.temporary_extractions(expires_at);

-- An inbox deletion or capture cleanup also removes the extraction snapshot.
CREATE FUNCTION public.delete_temporary_extraction_for_capture() RETURNS trigger
LANGUAGE plpgsql SET search_path = '' AS $$
BEGIN
    DELETE FROM public.temporary_extractions
    WHERE id IN (SELECT extraction_id FROM public.temporary_extraction_pages WHERE capture_id = OLD.id);
    RETURN OLD;
END;
$$;
REVOKE ALL ON FUNCTION public.delete_temporary_extraction_for_capture() FROM PUBLIC, anon, authenticated;
CREATE TRIGGER temporary_extraction_capture_deleted BEFORE DELETE ON public.captures
FOR EACH ROW EXECUTE FUNCTION public.delete_temporary_extraction_for_capture();

ALTER TABLE public.temporary_extractions ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.temporary_extraction_pages ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.temporary_extractions, public.temporary_extraction_pages FROM PUBLIC, anon, authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.temporary_extractions, public.temporary_extraction_pages TO service_role;
