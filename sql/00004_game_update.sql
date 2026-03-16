-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION game.update(game_id UUID, tick_ms float8) RETURNS void AS $$
    BEGIN
        RAISE NOTICE 'game.update()';
    END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.start_game(game_id UUID, player_name VARCHAR) RETURNS void AS $$
    BEGIN
        PERFORM game_loop.start_game(game_id, player_name, 'game.update');
    END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd