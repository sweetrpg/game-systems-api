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

## Rate limiting

Per-client/IP rate limiting is the platform default via the shared `api-core.go/ratelimit`
middleware (Redis-backed counters keyed by `X-API-Key` else client IP, `cheap` tier for
`/status/*`, fail-closed 503, 429 on exceed). Tune with `RATE_LIMIT_CHEAP`/
`RATE_LIMIT_CHEAP_WINDOW_SECONDS`/`RATE_LIMIT_STANDARD`/`RATE_LIMIT_STANDARD_WINDOW_SECONDS`.

The middleware is registered only when a Redis pool is configured (`REDIS_HOST` set); until the
`game-systems` namespace cache is provisioned in `support/infrastructure` it logs a startup WARN
and runs without limiting. A follow-up makes it unconditional once `REDIS_*` lands in the dev
overlay. See `platform`'s `openspec/changes/fix-rate-limiting-per-client-ip`.

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

## Platform Conventions and Decisions

This repo is a submodule of `sweetrpg/platform`. Platform-wide conventions and Architecture
Decision Records live there:

- checked out inside the platform tree: `../../docs/README.md` (convention index) and
  `../../docs/adr/README.md` (platform ADRs, `PADR-*`)
- standalone or module-only checkout:
  <https://github.com/sweetrpg/platform/tree/master/docs> and
  <https://github.com/sweetrpg/platform/tree/master/docs/adr>

Accepted `PADR-*` records are binding constraints. Read the ADR index before proposing a
structural change; if a task needs to contradict an accepted record, stop and say so -
propose a superseding ADR rather than working around it. This repo's own service-local
decisions are `ADR-*` and belong in `docs/adr/` here.

Before implementing any feature, complete the
[feature work checkpoint](https://github.com/sweetrpg/platform/blob/master/docs/openspec.md#feature-work-checkpoint)
and record the answers in the issue or PR.
