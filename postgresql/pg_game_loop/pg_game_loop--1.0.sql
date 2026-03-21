CREATE EXTENSION IF NOT EXISTS pg_cron;
CREATE SCHEMA game_loop;

CREATE TYPE game_loop.game_state AS ENUM ('RUNNING', 'FINISHED', 'INITIATED');

CREATE TABLE game_loop.games(
    id UUID PRIMARY KEY,
    player_name VARCHAR(255) NOT NULL,
    game_state game_loop.game_state,
    init_function VARCHAR(64) NOT NULL,
    update_function VARCHAR(64) NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE,
    last_update TIMESTAMP WITH TIME ZONE
);

CREATE FUNCTION game_loop.start_game(game_id UUID,
                                     player_name VARCHAR,
                                     init_function VARCHAR,
                                     update_function VARCHAR) RETURNS void AS $$
DECLARE
    app_name TEXT;
    current_ts TIMESTAMP WITH TIME ZONE;
BEGIN
    app_name := current_setting('application_name');
    IF app_name = '' THEN
        RAISE EXCEPTION 'application_name must be set';
    END IF;
    IF app_name::uuid <> game_id THEN
        RAISE EXCEPTION 'application_name must be the same as game_id';
    END IF;
    current_ts := NOW();
    INSERT INTO game_loop.games VALUES(game_id,
                                       player_name,
                                       'INITIATED',
                                       init_function,
                                       update_function,
                                       current_ts,
                                       current_ts);
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game_loop.finish_orphans() RETURNS void AS $$
DECLARE
    now_ts TIMESTAMPTZ := clock_timestamp();
    curs CURSOR FOR SELECT g.id as game_id, a.application_name as app_name
                      FROM game_loop.games g
                      LEFT JOIN pg_stat_activity a ON g.id::text = a.application_name
                      WHERE g.game_state = 'RUNNING';
BEGIN
    FOR r IN curs LOOP
        IF r.app_name IS NULL THEN
            RAISE NOTICE 'Game % orphaned, finishing...', r.game_id;
            UPDATE game_loop.games
            SET game_state = 'FINISHED', last_update = now_ts
            WHERE game_loop.games.id = r.game_id;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game_loop.delete_finished() RETURNS void AS $$
DECLARE
    now_ts TIMESTAMPTZ := clock_timestamp();
BEGIN
    DELETE FROM game_loop.games as g
           WHERE g.game_state = 'FINISHED' AND now_ts - g.last_update > interval '1 minute';
END;
$$ LANGUAGE plpgsql;

SELECT cron.schedule(
   'finish_orphans',
   '2 seconds',
   'SELECT game_loop.finish_orphans()'
);

SELECT cron.schedule(
   'delete_finished',
   '30 seconds',
   'SELECT game_loop.delete_finished()'
);
