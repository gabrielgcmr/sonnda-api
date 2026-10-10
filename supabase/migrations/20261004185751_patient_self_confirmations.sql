-- supabase/migrations/20261004185751_patient_self_confirmations.sql
-- A professional attestation is separate from legacy owner_user_id and self grants.
CREATE TABLE public.patient_self_confirmations (
    id uuid PRIMARY KEY,
    patient_id uuid NOT NULL REFERENCES public.patients(id),
    account_id uuid NOT NULL REFERENCES public.users(id),
    confirmed_by uuid NOT NULL REFERENCES public.users(id),
    confirmed_at timestamptz NOT NULL DEFAULT now(),
    revoked_by uuid REFERENCES public.users(id),
    revoked_at timestamptz,
    access_created boolean NOT NULL DEFAULT false,
    CONSTRAINT patient_self_confirmations_other_professional
      CHECK (confirmed_by <> account_id),
    CONSTRAINT patient_self_confirmations_revocation_pair
      CHECK ((revoked_by IS NULL) = (revoked_at IS NULL)),
    CONSTRAINT patient_self_confirmations_revocation_order
      CHECK (revoked_at IS NULL OR revoked_at >= confirmed_at)
);

CREATE UNIQUE INDEX patient_self_confirmations_active_patient
    ON public.patient_self_confirmations(patient_id) WHERE revoked_at IS NULL;
CREATE UNIQUE INDEX patient_self_confirmations_active_account
    ON public.patient_self_confirmations(account_id) WHERE revoked_at IS NULL;
CREATE INDEX patient_self_confirmations_account_patient
    ON public.patient_self_confirmations(account_id, patient_id);

ALTER TABLE public.patient_self_confirmations ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.patient_self_confirmations FROM PUBLIC, anon, authenticated;
