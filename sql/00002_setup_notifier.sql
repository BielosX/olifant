-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION tick() RETURNS void AS $$
    BEGIN
        RAISE NOTICE 'tick() called';
        NOTIFY game, 'Hello';
    END;
$$ LANGUAGE plpgsql;

SELECT cron.schedule(
   'tick',
   '10 seconds',
   'SELECT tick()'
);
-- +goose StatementEnd
