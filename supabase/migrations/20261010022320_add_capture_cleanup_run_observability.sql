-- supabase/migrations/20261010022320_add_capture_cleanup_run_observability.sql

create table public.capture_cleanup_runs (
    id uuid primary key,
    status text not null,
    captures_processed bigint not null default 0,
    captures_deleted bigint not null default 0,
    storage_deleted bigint not null default 0,
    sessions_deleted bigint not null default 0,
    error_count integer not null default 0,
    started_at timestamptz not null,
    finished_at timestamptz,
    constraint capture_cleanup_runs_status_check
        check (status in ('running', 'succeeded', 'failed')),
    constraint capture_cleanup_runs_counts_check
        check (
            captures_processed >= 0
            and captures_deleted >= 0
            and storage_deleted >= 0
            and sessions_deleted >= 0
            and error_count >= 0
        ),
    constraint capture_cleanup_runs_lifecycle_check
        check (
            (status = 'running' and finished_at is null)
            or
            (status in ('succeeded', 'failed') and finished_at is not null and finished_at >= started_at)
        )
);

create index capture_cleanup_runs_started_at_idx
    on public.capture_cleanup_runs(started_at desc);

create index captures_cleanup_order_idx
    on public.captures(expires_at, created_at, id);

alter table public.capture_cleanup_runs enable row level security;
revoke all on table public.capture_cleanup_runs from public, anon, authenticated;
grant select, insert, update on table public.capture_cleanup_runs to service_role;
