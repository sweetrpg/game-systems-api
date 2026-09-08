# SweetRPG Game Systems API

[![CI](https://github.com/sweetrpg/game-systems-api/actions/workflows/ci.yaml/badge.svg)](https://github.com/sweetrpg/game-systems-api/actions/workflows/ci.yaml)
[![License](https://img.shields.io/github/license/sweetrpg/game-systems-api.svg)](https://img.shields.io/github/license/sweetrpg/game-systems-api.svg)
[![Issues](https://img.shields.io/github/issues/sweetrpg/game-systems-api.svg)](https://img.shields.io/github/issues/sweetrpg/game-systems-api.svg)
[![PRs](https://img.shields.io/github/issues-pr/sweetrpg/game-systems-api.svg)](https://img.shields.io/github/issues-pr/sweetrpg/game-systems-api.svg)
[![Dependabot](https://badgen.net/github/dependabot/sweetrpg/game-systems-api)](https://badgen.net/github/dependabot/sweetrpg/game-systems-api)

HTTP microservice that owns the platform-wide catalog of game systems (ruleset/edition, name,
publisher reference, metadata). A Gin-based Go service, matching `catalog-api`'s conventions,
backed by its own MongoDB database.

`catalog-api` resolves a `Volume`'s system references against this service rather than
maintaining its own copy - see `sweetrpg/platform`'s `openspec/changes/game-systems-service` for
the full design.

## Run locally

```bash
go run ./cmd/game-systems-api
```

Needs a MongoDB instance reachable per `mongodb.go`'s `DB_*` environment variables (`DB_URI`, or
`DB_HOST`/`DB_SCHEME`/`DB_USER`/`DB_PW`/`DB_NAME`/`DB_PORT`/`DB_OPTS`).

## HTTP API

`GET /systems` lists live game systems, filtered/sorted/paged at the query layer:

| Param      | Default | Notes                                                        |
|------------|---------|-------------------------------------------------------------|
| `q`        | -       | Case-insensitive substring match on name                    |
| `sort`     | `name`  | One of `name`, `-name`, `created`, `-created`               |
| `page`     | `1`     | 1-based                                                      |
| `per_page` | `24`    | Hard max 100; larger values are clamped, not rejected       |

Response:

```json
{ "systems": [ /* flattened current views */ ], "total": 0, "page": 1, "per_page": 24 }
```

A non-integer `page`/`per_page` or an off-allowlist `sort` returns `400`. With no params the
endpoint returns the first page under the default sort and size.

## Documentation

Package documentation: [pkg.go.dev/github.com/sweetrpg/game-systems-api](https://pkg.go.dev/github.com/sweetrpg/game-systems-api).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow.
