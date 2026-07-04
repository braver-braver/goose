-- +goose Up
ALTER TABLE repos ADD (
    homepage_url VARCHAR2(2000),
    is_private NUMBER(1) DEFAULT 0 NOT NULL
);

-- +goose Down
ALTER TABLE repos DROP (homepage_url, is_private);
