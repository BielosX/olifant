-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA game;

CREATE TYPE game.inputs AS ENUM (
    'UP_PRESSED',
    'UP_RELEASED',
    'DOWN_PRESSED',
    'DOWN_RELEASED',
    'LEFT_PRESSED',
    'LEFT_RELEASED',
    'RIGHT_PRESSED',
    'RIGHT_RELEASED'
);

CREATE TABLE game.input_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id UUID REFERENCES game_loop.games (id) ON DELETE CASCADE,
    order_number BIGINT NOT NULL DEFAULT 0,
    event game.inputs,
    UNIQUE (game_id, order_number)
);

CREATE FUNCTION game.set_order_number() RETURNS TRIGGER AS $$
BEGIN
    SELECT COALESCE(MAX(order_number), 0) + 1
    INTO NEW.order_number
    FROM game.input_events
    WHERE game_id = NEW.game_id;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_set_order_number
    BEFORE INSERT ON game.input_events
    FOR EACH ROW EXECUTE FUNCTION game.set_order_number();

CREATE TYPE game.keysState AS (
    _up boolean,
    _down boolean,
    _left boolean,
    _right boolean
);

CREATE TABLE game.players (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id UUID UNIQUE REFERENCES game_loop.games (id) ON DELETE CASCADE,
    position vec.vec2,
    velocity vec.vec2,
    keysPressed game.keysState,
    score INTEGER
);

CREATE TABLE game.consts (
    key CHAR(64) PRIMARY KEY,
    value JSONB
);

INSERT INTO game.consts (key, value) VALUES
    ('bounding_circle_radius', '{"player": 0.1}'::jsonb);
-- +goose StatementEnd
