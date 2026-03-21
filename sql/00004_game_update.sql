-- +goose Up
-- +goose StatementBegin

CREATE FUNCTION game.init(game_id UUID) RETURNS void AS $$
BEGIN
    INSERT INTO game.players (game_id, position, velocity, score) VALUES
        (game_id, ARRAY[0.5, 0.5], vec.vec2_zero(), 0);
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.update(game_id UUID, tick_ms float8) RETURNS boolean AS $$
BEGIN
    RAISE NOTICE 'game.update()';
    RETURN false;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.start_game(game_id UUID, player_name VARCHAR) RETURNS void AS $$
BEGIN
    PERFORM game_loop.start_game(game_id, player_name, 'game.init', 'game.update');
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd