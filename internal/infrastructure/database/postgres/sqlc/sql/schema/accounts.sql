-- internal/infrastructure/database/postgres/sqlc/sql/schema/accounts.sql
CREATE TABLE accounts (
  id UUID PRIMARY KEY,
  full_name TEXT,
  birth_date DATE,
  cpf TEXT UNIQUE,
  phone TEXT,
  account_type TEXT NOT NULL DEFAULT 'basic_care' CHECK (account_type IN ('professional', 'basic_care')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_accounts_deleted_at ON accounts (deleted_at);
CREATE TABLE account_identities (
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  issuer TEXT NOT NULL,
  subject TEXT NOT NULL,
  email TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (issuer, subject),
  CHECK (btrim(issuer) <> ''),
  CHECK (btrim(subject) <> '')
);
CREATE INDEX account_identities_account_id_idx ON account_identities (account_id);
