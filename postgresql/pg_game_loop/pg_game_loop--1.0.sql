CREATE SCHEMA game_loop;

CREATE FUNCTION game_loop.run() RETURNS void
    AS 'pg_game_loop', 'game_loop_run'
    LANGUAGE C STRICT;