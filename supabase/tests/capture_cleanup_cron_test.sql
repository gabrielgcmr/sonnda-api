-- supabase/tests/capture_cleanup_cron_test.sql

begin;
select plan(12);

select ok(
    exists (
        select 1
        from pg_extension extension
        join pg_namespace namespace on namespace.oid = extension.extnamespace
        where extension.extname = 'pg_cron'
          and namespace.nspname = 'pg_catalog'
    ),
    'pg_cron is installed in pg_catalog'
);

select ok(
    exists (
        select 1
        from pg_extension extension
        join pg_namespace namespace on namespace.oid = extension.extnamespace
        where extension.extname = 'pg_net'
          and namespace.nspname = 'extensions'
    ),
    'pg_net is installed in extensions'
);

select is(
    (select count(*) from cron.job where jobname = 'cleanup-captures-hourly'),
    1::bigint,
    'exactly one capture cleanup job exists'
);

select is(
    (select active from cron.job where jobname = 'cleanup-captures-hourly'),
    true,
    'capture cleanup job is active'
);

select is(
    (select schedule from cron.job where jobname = 'cleanup-captures-hourly'),
    '0 * * * *',
    'capture cleanup job runs hourly'
);

select ok(
    position('net.http_post' in (select command from cron.job where jobname = 'cleanup-captures-hourly')) > 0,
    'capture cleanup job performs an HTTP POST'
);

select ok(
    position('vault.decrypted_secrets' in (select command from cron.job where jobname = 'cleanup-captures-hourly')) > 0,
    'capture cleanup job reads secrets from Vault'
);

select ok(
    position('capture_cleanup_url' in (select command from cron.job where jobname = 'cleanup-captures-hourly')) > 0,
    'capture cleanup job reads the URL secret'
);

select ok(
    position('capture_cleanup_token' in (select command from cron.job where jobname = 'cleanup-captures-hourly')) > 0,
    'capture cleanup job reads the token secret'
);

select ok(
    position('X-Cleanup-Token' in (select command from cron.job where jobname = 'cleanup-captures-hourly')) > 0,
    'capture cleanup job sends the dedicated token header'
);

select ok(
    (select command from cron.job where jobname = 'cleanup-captures-hourly') !~* 'https?://',
    'capture cleanup job does not hardcode an endpoint URL'
);

select ok(
    position('timeout_milliseconds := 550000' in (select command from cron.job where jobname = 'cleanup-captures-hourly')) > 0,
    'capture cleanup HTTP request stays below the ten-minute cron limit'
);

select * from finish();
rollback;
