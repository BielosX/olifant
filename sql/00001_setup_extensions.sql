-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pg_cron;
CREATE EXTENSION IF NOT EXISTS pg_game_loop;
-- +goose StatementEnd
