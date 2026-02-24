-- +goose Up
-- +goose StatementBegin
CREATE TABLE input_events(
    id INTEGER GENERATED ALWAYS AS IDENTITY
        (START WITH 1 INCREMENT BY 1 CYCLE),
    event VARCHAR(64)
);
-- +goose StatementEnd
