-- supabase/migrations/20261009210052_schedule_capture_cleanup.sql

create extension if not exists pg_cron with schema pg_catalog;
create extension if not exists pg_net with schema extensions;

-- Provision these secrets before the first execution:
--   capture_cleanup_url: the absolute API URL ending in /internal/jobs/cleanup-captures
--   capture_cleanup_token: the same value configured as CAPTURE_CLEANUP_TOKEN in the API
select cron.schedule(
    'cleanup-captures-hourly',
    '0 * * * *',
    $job$
    select net.http_post(
        url := (
            select decrypted_secret
            from vault.decrypted_secrets
            where name = 'capture_cleanup_url'
            limit 1
        ),
        headers := jsonb_build_object(
            'Content-Type', 'application/json',
            'X-Cleanup-Token', (
                select decrypted_secret
                from vault.decrypted_secrets
                where name = 'capture_cleanup_token'
                limit 1
            )
        ),
        body := '{}'::jsonb,
        timeout_milliseconds := 550000
    );
    $job$
);
