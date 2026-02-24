-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION pg_cron;
CREATE EXTENSION pg_game_loop;
-- +goose StatementEnd
