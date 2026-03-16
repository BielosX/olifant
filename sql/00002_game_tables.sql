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
-- +goose StatementEnd
