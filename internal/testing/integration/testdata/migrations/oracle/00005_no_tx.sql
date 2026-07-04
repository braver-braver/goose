-- +goose NO TRANSACTION

-- +goose Up
CREATE UNIQUE INDEX owners_owner_name_idx ON owners(owner_name);

-- +goose Down
DROP INDEX owners_owner_name_idx;
