# AGENTS.md

This file provides guidance to Claude Code, Codex, GitHub Copilot, and other AI coding agents
working in this repository.

## About This Project

`game-systems-api` is the SweetRPG platform's system of record for game systems (ruleset/edition,
name, publisher reference, metadata). It replaces a retired Swift/Vapor scaffold - the service is
being rebuilt as a Go module (Gin + `api-core.go`), matching `catalog-api`'s conventions, backed
by its own MongoDB database. See `sweetrpg/platform`'s `openspec/changes/game-systems-service`
for the full design and task breakdown.

`catalog-api` resolves a `Volume`'s system references against this service over HTTP instead of
storing its own copy - `game-systems-api` has no reverse dependency on `catalog-api`.

## HTTP API notes

`GET /systems` is search/sort/paginated at the query layer: `q` (case-insensitive name
substring), `sort` (`name`/`-name`/`created`/`-created`), `page`, `per_page` (default 24, max
100, clamped). It returns `{ systems, total, page, per_page }`. `models.List` runs this as one
aggregation on `game_systems_versions` (`$match` state=live + optional regex, `$lookup`
`game_systems_meta`, `$match` no `deleted_at`, `$facet` page + count) - the meta-side soft-delete
filter is why it is a `$lookup` and not a version-only skip/limit. Off-allowlist `sort` yields
`models.ErrInvalidSort`; the handler maps that and unparseable pagination to `400`.

## Committing Code

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

## Branches and Workflow

* `develop` - integration branch, default branch, target for all PRs.
* `master` - latest released state, nothing committed directly.
* `feature/*`, `fix/*` branched from `develop`; `hotfix/*` branched from `master`.

See `CONTRIBUTING.md` for the full workflow.

## Running Checks Locally

```bash
go build -v ./...
go vet ./...
go test -v -coverprofile coverage.out ./...
```
