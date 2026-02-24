CREATE SCHEMA game_loop;

CREATE FUNCTION game_loop.run() RETURNS void AS $$
    BEGIN
        RAISE NOTICE 'game_loop started';
    END;
$$ LANGUAGE plpgsql;