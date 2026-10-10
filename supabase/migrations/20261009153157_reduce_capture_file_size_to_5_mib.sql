-- supabase/migrations/20261009153157_reduce_capture_file_size_to_5_mib.sql
ALTER TABLE public.captures
    DROP CONSTRAINT captures_size;

ALTER TABLE public.captures
    ADD CONSTRAINT captures_size
    CHECK (size_bytes > 0 AND size_bytes <= 5242880)
    NOT VALID;

ALTER TABLE public.captures
    VALIDATE CONSTRAINT captures_size;
