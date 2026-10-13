-- supabase/migrations/20261013011351_temporary_extraction_jobs.sql
ALTER TABLE public.temporary_extractions
    ALTER COLUMN result DROP NOT NULL,
    ADD COLUMN status text NOT NULL DEFAULT 'completed'
        CHECK (status IN ('queued', 'processing', 'completed', 'failed')),
    ADD COLUMN progress smallint NOT NULL DEFAULT 100 CHECK (progress BETWEEN 0 AND 100),
    ADD COLUMN error_message text,
    ADD COLUMN lease_owner uuid,
    ADD COLUMN lease_until timestamptz,
    ADD COLUMN attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    ADD CONSTRAINT temporary_extractions_result_state
        CHECK ((status = 'completed') = (result IS NOT NULL));

CREATE INDEX temporary_extractions_claim_idx
    ON public.temporary_extractions(status, lease_until, created_at)
    WHERE status IN ('queued', 'processing');
