BEGIN;
DO $body$ BEGIN PERFORM pg_sleep(0); END $body$;
COMMIT;
