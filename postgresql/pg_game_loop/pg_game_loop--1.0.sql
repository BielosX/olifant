CREATE SCHEMA game_loop;

CREATE FUNCTION game_loop.run(game_id UUID, entry TEXT) RETURNS void
    AS 'pg_game_loop', 'game_loop_run'
LANGUAGE C STRICT;