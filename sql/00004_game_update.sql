-- +goose Up
-- +goose StatementBegin

CREATE FUNCTION game.init(game_id UUID) RETURNS void AS $$
BEGIN
    INSERT INTO game.players (game_id, position, velocity, direction, keysPressed, score) VALUES
        (game_id,
         vec.vec2_of(0.5, 0.5),
         vec.vec2_zero(),
         vec.vec2_up(),
         ROW(false, false, false, false),
         0);
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.clamp(val float8, min_value float8, max_value float8) RETURNS float8 AS $$
BEGIN
    RETURN GREATEST(min_value, LEAST(val, max_value));
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.map_event(event game.inputs, curr game.keysState) RETURNS game.keysState AS $$
DECLARE
    r game.keysState;
BEGIN
    r := curr;
    CASE event
        WHEN 'UP_PRESSED' THEN r._up := true;
        WHEN 'UP_RELEASED' THEN r._up := false;
        WHEN 'DOWN_PRESSED' THEN r._down := true;
        WHEN 'DOWN_RELEASED' THEN r._down := false;
        WHEN 'LEFT_PRESSED' THEN r._left := true;
        WHEN 'LEFT_RELEASED' THEN r._left := false;
        WHEN 'RIGHT_PRESSED' THEN r._right := true;
        WHEN 'RIGHT_RELEASED' THEN r._right := false;
    END CASE;
    RETURN r;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.keys_to_velocity(keys game.keysState) RETURNS vec.vec2 AS $$
DECLARE
    r vec.vec2;
BEGIN
    r := vec.vec2_zero();
    IF keys._up THEN
       r := vec.vec2_normalize(vec.vec2_add(r, vec.vec2_up()));
    END IF;
    IF keys._down THEN
       r := vec.vec2_normalize(vec.vec2_add(r, vec.vec2_down()));
    END IF;
    IF keys._left THEN
       r := vec.vec2_normalize(vec.vec2_add(r, vec.vec2_left()));
    END IF;
    IF keys._right THEN
       r := vec.vec2_normalize(vec.vec2_add(r, vec.vec2_right()));
    END IF;
    RETURN r;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.update(v_game_id UUID, tick_ms float8) RETURNS boolean AS $$
DECLARE
    tick_s float8;
    player game.players%ROWTYPE;
    keys game.keysState;
    v_velocity vec.vec2;
    v_direction vec.vec2;
    v_position vec.vec2;
    events CURSOR FOR SELECT * FROM game.input_events
                               WHERE game_id=v_game_id ORDER BY order_number ASC LIMIT 10 FOR UPDATE;
BEGIN
    tick_s := tick_ms / 1000.0;
    SELECT * INTO player FROM game.players WHERE game_id=v_game_id;
    keys := player.keysPressed;
    FOR e in events LOOP
        keys := game.map_event(e.event::game.inputs, keys);
        DELETE FROM game.input_events WHERE CURRENT OF events;
    END LOOP;
    v_velocity := vec.vec2_mul_scalar(game.keys_to_velocity(keys), 0.3);
    v_velocity := vec.vec2_mul_scalar(v_velocity, tick_s);
    v_position := vec.vec2_add(player.position, v_velocity);
    IF vec.vec2_len(v_velocity) > 1e-6 THEN
        v_direction := vec.vec2_normalize(v_velocity);
    ELSE
        v_direction := player.direction;
    END IF;
    UPDATE game.players
        SET velocity=v_velocity, position=v_position, keysPressed=keys, direction=v_direction
        WHERE id=player.id;
    RETURN false;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.start_game(game_id UUID, player_name VARCHAR) RETURNS void AS $$
BEGIN
    PERFORM game_loop.start_game(game_id, player_name, 'game.init', 'game.update');
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd