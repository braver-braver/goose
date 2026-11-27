# Maintaining the Oracle-support fork

This fork deliberately has **two long-lived branches**:

| Branch | Module path | Maintenance policy |
| --- | --- | --- |
| [`main`](https://github.com/braver-braver/goose/tree/main) | `github.com/pressly/goose/v3` | Mirror the official [`pressly/goose/main`](https://github.com/pressly/goose/tree/main) branch without fork-specific patches. |
| [`oracle-support`](https://github.com/braver-braver/goose/tree/oracle-support) | `github.com/braver-braver/goose/v3` | Maintain the Oracle driver, dialect, migration behavior, and DBMS_LOCK integration used by this fork's consumers. |

Do **not** merge `oracle-support` back into `main`, rewrite the Oracle branch to match upstream, or rename the fork's Go module to the upstream path. Upstream changes should reach `oracle-support` only through reviewed, tested integration pull requests based on the Oracle branch. Keep the repository's default branch set to `main`.

## Install and consume the maintained Oracle branch

Install the fork CLI (requires a suitable Go toolchain):

```sh
go install github.com/braver-braver/goose/v3/cmd/goose@oracle-support
```

Use the fork as a library:

```sh
go get github.com/braver-braver/goose/v3@oracle-support
```

Import `github.com/braver-braver/goose/v3` in application source. **For deployed applications, pin an already tested commit or resolved pseudo-version** rather than tracking the moving `oracle-support` ref.

The `github.com/pressly/goose/v3` installation command and the `goose` Homebrew formula install **upstream**, not this Oracle fork. Do not use them when the project depends on the fork-specific Oracle implementation.

## Rules when incorporating upstream updates

1. Update `main` only to an upstream `pressly/goose/main` commit without mixing in Oracle-only changes.
2. Compare `main` and `oracle-support` and review upstream changes in small groups; a divergent history is expected.
3. Create an integration branch **from `oracle-support`** and open a PR back into `oracle-support`; do not bypass its CI.
4. Preserve the fork module path, the existing `goose_db_version` structure and migration semantics, Oracle SQL normalization, and session-lock behavior. Treat any change to these contracts as a separate compatibility review.
5. Require formatting, vet, build, unit tests, lint, and Docker integration tests to pass before merging. Verify Oracle-specific migrations and locking explicitly.

## Oracle compatibility checklist

The automated Docker test environment uses **Oracle Database Free 23.x**, **not Oracle Database 19c**. A green CI run is therefore not evidence of full Oracle 19c compatibility. For a production-facing change, validate on an isolated Oracle 19c database or test schema:

- Initialization and reading of `goose_db_version`, including existing migration history.
- Migration `up`, `down`, `status`, and rerunning after a failed migration.
- SQL statement normalization and PL/SQL blocks with `-- +goose StatementBegin` / `-- +goose StatementEnd`.
- Oracle DDL implicit-commit behavior; do not assume an entire multi-DDL migration rolls back. Keep each migration file to one DDL statement when possible.
- Concurrent migration execution with `DBMS_LOCK`, including physical session termination. The locking user must have `EXECUTE` on `SYS.DBMS_LOCK`.

On the Oracle branch, the available verification commands include:

```sh
make add-gowork
go vet ./...
go build ./...
make test-packages
make test-oracle
make test-integration
```

The full integration suite requires Docker and starts several database containers. GitHub Actions run the integration suite serially to avoid resource contention.

## Releases

The release workflow triggers on tags. Do not tag a release or promise prebuilt binaries merely because `main` or an Oracle PR passes CI. Review the relevant branch, packaging configuration, and Oracle 19c verification before publishing an Oracle-specific build.
