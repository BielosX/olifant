-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA game;

CREATE FUNCTION game.tick(game_id UUID, tick_ms float8) RETURNS void AS $$
    BEGIN
        RAISE NOTICE 'game.tick()';
    END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.run(game_id UUID) RETURNS void AS $$
    BEGIN
        SELECT game_loop.run(game_id, 'game.tick');
    END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd