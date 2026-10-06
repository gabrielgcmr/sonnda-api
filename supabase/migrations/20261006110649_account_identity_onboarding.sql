-- supabase/migrations/20261006110649_account_identity_onboarding.sql
-- Renaming the table keeps its primary key and every existing foreign key intact.
ALTER TABLE public.users RENAME TO accounts;

CREATE TABLE public.account_identities (
    account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
    issuer text NOT NULL,
    subject text NOT NULL,
    email text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_identities_issuer_subject_key UNIQUE (issuer, subject),
    CONSTRAINT account_identities_issuer_not_blank CHECK (btrim(issuer) <> ''),
    CONSTRAINT account_identities_subject_not_blank CHECK (btrim(subject) <> '')
);

CREATE INDEX account_identities_account_id_idx
    ON public.account_identities(account_id);

INSERT INTO public.account_identities
    (account_id, issuer, subject, email, created_at, updated_at)
SELECT id, auth_issuer, auth_subject, email, created_at, updated_at
FROM public.accounts;

-- Fail the migration before removing the source columns if any row was not copied.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.accounts a
        WHERE NOT EXISTS (
            SELECT 1 FROM public.account_identities i
            WHERE i.account_id = a.id
              AND i.issuer = a.auth_issuer
              AND i.subject = a.auth_subject
              AND i.email IS NOT DISTINCT FROM a.email
        )
    ) THEN
        RAISE EXCEPTION 'account identity backfill incomplete';
    END IF;
END;
$$;

DROP POLICY IF EXISTS "Users can CRUD own profile" ON public.accounts;
DROP INDEX IF EXISTS public.idx_users_email;
DROP INDEX IF EXISTS public.idx_users_auth_identity;
ALTER TABLE public.accounts DROP CONSTRAINT IF EXISTS users_email_key;
ALTER TABLE public.accounts
    DROP COLUMN auth_issuer,
    DROP COLUMN auth_subject,
    DROP COLUMN email,
    ALTER COLUMN full_name DROP NOT NULL,
    ALTER COLUMN birth_date DROP NOT NULL,
    ALTER COLUMN cpf DROP NOT NULL,
    ALTER COLUMN phone DROP NOT NULL;

-- The existing non-partial CPF unique constraint also covers deactivated accounts.
ALTER TABLE public.accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.account_identities ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.accounts, public.account_identities
    FROM PUBLIC, anon, authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.accounts, public.account_identities
    TO service_role;

-- The definer lookup is only for patient RLS, outside the exposed public schema.
CREATE SCHEMA IF NOT EXISTS private;
ALTER FUNCTION public.current_app_user_id() SET SCHEMA private;
CREATE OR REPLACE FUNCTION private.current_app_user_id()
RETURNS uuid
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $$
    SELECT a.id
    FROM public.account_identities i
    JOIN public.accounts a ON a.id = i.account_id
    WHERE i.issuer = (SELECT auth.jwt() ->> 'iss')
      AND i.subject = (SELECT auth.jwt() ->> 'sub')
      AND a.deleted_at IS NULL
      AND char_length(btrim(a.full_name)) BETWEEN 2 AND 120
      AND a.birth_date IS NOT NULL
      AND a.birth_date <= CURRENT_DATE
    LIMIT 1
$$;
REVOKE ALL ON FUNCTION private.current_app_user_id() FROM PUBLIC, anon;
GRANT USAGE ON SCHEMA private TO authenticated, service_role;
GRANT EXECUTE ON FUNCTION private.current_app_user_id() TO authenticated, service_role;

DROP POLICY "users can create patients" ON public.patients;
CREATE POLICY "users can create patients" ON public.patients
    FOR INSERT TO authenticated
    WITH CHECK (
        created_by_user_id = (SELECT private.current_app_user_id())
        AND (owner_user_id IS NULL OR owner_user_id = (SELECT private.current_app_user_id()))
    );
DROP POLICY "users can view linked patients" ON public.patients;
CREATE POLICY "users can view linked patients" ON public.patients
    FOR SELECT TO authenticated
    USING (EXISTS (
        SELECT 1 FROM public.patient_access pa
        WHERE pa.patient_id = patients.id
          AND pa.grantee_id = (SELECT private.current_app_user_id())
          AND pa.revoked_at IS NULL
    ));
