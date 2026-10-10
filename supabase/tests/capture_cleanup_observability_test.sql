-- supabase/tests/capture_cleanup_observability_test.sql

begin;
select plan(8);

select ok(
    to_regclass('public.capture_cleanup_runs') is not null,
    'capture cleanup runs table exists'
);

select is(
    (select relrowsecurity from pg_class where oid = 'public.capture_cleanup_runs'::regclass),
    true,
    'capture cleanup runs has RLS enabled'
);

select is(
    has_table_privilege('anon', 'public.capture_cleanup_runs', 'SELECT'),
    false,
    'anon cannot read cleanup runs'
);

select is(
    has_table_privilege('authenticated', 'public.capture_cleanup_runs', 'SELECT'),
    false,
    'authenticated cannot read cleanup runs'
);

select ok(
    has_table_privilege('service_role', 'public.capture_cleanup_runs', 'SELECT')
    and has_table_privilege('service_role', 'public.capture_cleanup_runs', 'INSERT')
    and has_table_privilege('service_role', 'public.capture_cleanup_runs', 'UPDATE'),
    'service role can record and inspect cleanup runs'
);

select is(
    (
        select count(*)
        from pg_constraint
        where conrelid = 'public.capture_cleanup_runs'::regclass
          and conname in (
              'capture_cleanup_runs_status_check',
              'capture_cleanup_runs_counts_check',
              'capture_cleanup_runs_lifecycle_check'
          )
    ),
    3::bigint,
    'cleanup run lifecycle constraints exist'
);

select ok(
    to_regclass('public.capture_cleanup_runs_started_at_idx') is not null,
    'cleanup runs can be inspected by most recent start time'
);

select ok(
    to_regclass('public.captures_cleanup_order_idx') is not null,
    'capture cleanup keyset order has a supporting index'
);

select * from finish();
rollback;
