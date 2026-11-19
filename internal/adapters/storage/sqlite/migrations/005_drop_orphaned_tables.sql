-- +goose Up
-- +goose StatementBegin
DROP TABLE IF EXISTS article_teams;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS teams;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE teams (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE article_teams (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    article_id INTEGER NOT NULL,
    team_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd