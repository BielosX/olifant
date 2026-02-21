-- +goose Up
-- +goose StatementBegin
CREATE PROCEDURE tick()
LANGUAGE plpgsql
AS $$
    BEGIN
        RAISE NOTICE 'tick() called';
        NOTIFY game, 'Hello';
    END;
$$;

SELECT cron.schedule(
   'tick',
   '10 seconds',
   'CALL tick()'
);
-- +goose StatementEnd
