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


CREATE FUNCTION game.generate_enemy_spawn_point() RETURNS vec.vec2 AS $$
DECLARE
    fixed_axis integer;
    axis_value float8;
BEGIN
    fixed_axis := random(0, 3);
    axis_value := random();
    CASE fixed_axis
        WHEN 0 THEN RETURN vec.vec2_of(0.0, axis_value);
        WHEN 1 THEN RETURN vec.vec2_of(1.0, axis_value);
        WHEN 2 THEN RETURN vec.vec2_of(axis_value, 0.0);
        WHEN 3 THEN RETURN vec.vec2_of(axis_value, 1.0);
    END CASE;
    RETURN vec.vec2_of(0.0, 0.0);
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.clamp(val float8, min_value float8, max_value float8, margin float8) RETURNS float8 AS $$
BEGIN
    IF val + margin > max_value THEN
       RETURN max_value - margin;
    END IF;
    IF val - margin < min_value THEN
       RETURN min_value + margin;
    END IF;
    RETURN val;
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

CREATE FUNCTION game.enemy_velocity(player game.players, enemy_position vec.vec2) RETURNS vec.vec2 AS $$
DECLARE
    enemy_speed float8 := 0.1;
BEGIN
    RETURN vec.vec2_mul_scalar(vec.vec2_normalize(vec.vec2_sub(player.position, enemy_position)), enemy_speed);
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.new_enemy(player game.players) RETURNS void AS $$
DECLARE
    max_enemies integer;
    enemies_count integer;
    last_enemy_created timestamp;
    enemy_position vec.vec2;
    enemy_velocity vec.vec2;
    hit_points integer;
    v_game_id UUID;
    now_ts timestamp;
BEGIN
    v_game_id := player.game_id;
    SELECT COALESCE(current_setting('game.max_enemies', true)::integer, 10) INTO max_enemies;
    SELECT count(*) INTO enemies_count FROM game.enemies WHERE game_id=v_game_id;
    SELECT COALESCE(MAX(created), '-infinity'::timestamp) INTO last_enemy_created FROM game.enemies WHERE game_id=v_game_id;
    enemy_position := game.generate_enemy_spawn_point();
    enemy_velocity := game.enemy_velocity(player, enemy_position);
    hit_points := random(1, 3);
    now_ts := clock_timestamp();
    RAISE NOTICE 'Spawned enemies: %, max enemies: %', enemies_count, max_enemies;
    IF enemies_count < max_enemies THEN
       IF now_ts - last_enemy_created > interval '0.5 seconds' THEN
           INSERT INTO game.enemies (game_id, position, velocity, hit_points, created)
           VALUES (player.game_id, enemy_position, enemy_velocity, hit_points, now_ts);
           RAISE NOTICE 'Enemy spawned at (%,%)', enemy_position[1], enemy_position[2];
       ELSE
        RAISE NOTICE 'Too early to spawn new enemy. Last spawn: %', last_enemy_created;
       END IF;
    ELSE
        RAISE NOTICE 'Max enemies spawned. Skip';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION game.update(v_game_id UUID, tick_ms float8) RETURNS boolean AS $$
DECLARE
    tick_s float8;
    bounding_radius jsonb;
    player_bounding_radius float8;
    enemy_bounding_radius float8;
    player game.players%ROWTYPE;
    keys game.keysState;
    v_velocity vec.vec2;
    v_direction vec.vec2;
    v_position vec.vec2;
    enemy_position vec.vec2;
    enemy_velocity vec.vec2;
    player_to_enemy_distance float8;
    player_speed float8 := 0.3;
    events CURSOR FOR SELECT * FROM game.input_events
                               WHERE game_id=v_game_id ORDER BY order_number ASC LIMIT 10 FOR UPDATE;
    enemies CURSOR FOR SELECT * FROM game.enemies WHERE game_id=v_game_id FOR UPDATE;
BEGIN
    tick_s := tick_ms / 1000.0;
    SELECT value
    INTO bounding_radius
    FROM game.consts
    WHERE key='bounding_circle_radius';
    player_bounding_radius := (bounding_radius->>'player')::float8;
    enemy_bounding_radius := (bounding_radius->>'enemy')::float8;
    SELECT * INTO player FROM game.players WHERE game_id=v_game_id;
    PERFORM game.new_enemy(player);
    FOR e in enemies LOOP
        enemy_velocity := e.velocity::vec.vec2;
        enemy_position := e.position::vec.vec2;
        player_to_enemy_distance := vec.vec2_len(vec.vec2_sub(player.position, enemy_position));
        IF player_to_enemy_distance < player_bounding_radius + enemy_bounding_radius THEN
           RETURN true;
        END IF;
        enemy_position := vec.vec2_add(vec.vec2_mul_scalar(enemy_velocity, tick_s), enemy_position);
        enemy_velocity := game.enemy_velocity(player, enemy_position);
        UPDATE game.enemies SET position=enemy_position, velocity=enemy_velocity WHERE CURRENT OF enemies;
    END LOOP;
    keys := player.keysPressed;
    FOR e in events LOOP
        keys := game.map_event(e.event::game.inputs, keys);
        DELETE FROM game.input_events WHERE CURRENT OF events;
    END LOOP;
    v_velocity := vec.vec2_mul_scalar(game.keys_to_velocity(keys), player_speed);
    v_position := vec.vec2_add(player.position, vec.vec2_mul_scalar(v_velocity, tick_s));
    v_position[1] = game.clamp(v_position[1], 0.0, 1.0, player_bounding_radius);
    v_position[2] = game.clamp(v_position[2], 0.0, 1.0, player_bounding_radius);
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