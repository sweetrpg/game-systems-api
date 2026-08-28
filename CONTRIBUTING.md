# Contributing

Thanks for considering a contribution to `game-systems-api`.

## Branching

This repo follows the sweetrpg platform's git-flow convention:

* `develop` is the integration branch. All feature and fix branches merge here.
* `master` reflects the latest released state. Nothing is committed here directly.
* Branch names: `feature/<description>` for new functionality, `fix/<description>` for bug
  fixes, `hotfix/<description>` for urgent fixes to a released version.

```bash
git checkout develop
git pull
git checkout -b feature/my-change
# ... work, commit ...
git push -u origin feature/my-change
# open a PR: feature/my-change -> develop
```

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

## Running checks locally

```bash
go build -v ./...
go vet ./...
go test -v -coverprofile coverage.out ./...
```

## Pull requests

CI runs automatically on PRs targeting `develop`. Once checks pass and the PR is reviewed, it
can be merged (auto-merge is enabled once required checks pass).

## Releases

Releases are cut via the `Prepare Release` workflow (`go-prepare-release.yaml`), which opens a
release PR; merging it into `master` triggers `go-tag-release.yaml` and `go-release.yaml` to tag
and publish the release. See a sibling like `admin-api` for the reference pattern.
