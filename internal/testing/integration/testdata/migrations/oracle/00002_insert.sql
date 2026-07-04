-- +goose Up
INSERT INTO owners (owner_name, owner_type) VALUES ('lucas', 'user');
INSERT INTO owners (owner_name, owner_type) VALUES ('space', 'organization');
INSERT INTO owners (owner_name, owner_type) VALUES ('james', 'user');
INSERT INTO owners (owner_name, owner_type) VALUES ('pressly', 'organization');

INSERT INTO repos (repo_full_name, repo_owner_id) VALUES ('james/rover', 3);
INSERT INTO repos (repo_full_name, repo_owner_id) VALUES ('pressly/goose', 4);

-- +goose Down
DELETE FROM owners;
