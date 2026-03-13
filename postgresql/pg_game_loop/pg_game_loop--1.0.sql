CREATE EXTENSION IF NOT EXISTS pg_cron;
CREATE SCHEMA game_loop;

CREATE TYPE game_loop.game_state AS ENUM ('RUNNING', 'FINISHED', 'INITIATED');

CREATE TABLE game_loop.games(
    id UUID PRIMARY KEY,
    player_name VARCHAR(255) NOT NULL,
    game_state game_loop.game_state,
    update_function VARCHAR(64) NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE,
    last_update TIMESTAMP WITH TIME ZONE
);

CREATE FUNCTION game_loop.start_game(game_id UUID, player_name VARCHAR, update_function VARCHAR) RETURNS void AS $$
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
    INSERT INTO game_loop.games VALUES(game_id, player_name, 'INITIATED', update_function, current_ts, current_ts);
END;
$$ LANGUAGE plpgsql;