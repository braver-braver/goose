-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE PROCEDURE insert_repository(
    p_repo_full_name IN VARCHAR2,
    p_owner_name     IN VARCHAR2,
    p_owner_type     IN VARCHAR2
) AS
    v_owner_id NUMBER;
BEGIN
    BEGIN
        SELECT owner_id INTO v_owner_id
        FROM owners
        WHERE owner_name = p_owner_name AND owner_type = p_owner_type;
    EXCEPTION
        WHEN NO_DATA_FOUND THEN
            INSERT INTO owners (owner_name, owner_type)
            VALUES (p_owner_name, p_owner_type)
            RETURNING owner_id INTO v_owner_id;
    END;

    INSERT INTO repos (repo_full_name, repo_owner_id)
    VALUES (p_repo_full_name, v_owner_id);
END insert_repository;
-- +goose StatementEnd

-- +goose Down
DROP PROCEDURE insert_repository;
