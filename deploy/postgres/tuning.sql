-- Optional PostgreSQL tuning for auction workload (run as superuser on VPS).
-- Tune shared_buffers / work_mem to ~25% RAM on small VPS; adjust for your machine.

-- ALTER SYSTEM SET shared_buffers = '256MB';
-- ALTER SYSTEM SET effective_cache_size = '1GB';
-- ALTER SYSTEM SET work_mem = '8MB';
-- ALTER SYSTEM SET maintenance_work_mem = '64MB';
-- ALTER SYSTEM SET random_page_cost = 1.1;
-- ALTER SYSTEM SET effective_io_concurrency = 200;
-- SELECT pg_reload_conf();
