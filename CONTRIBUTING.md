# Contributing

Thanks for your interest in improving `terraform-provider-azsqlaccess`! 🎉
Contributions — bug reports, docs, tests, and code — are welcome.

This guide covers **how to contribute and how we work**: the fork-and-PR
workflow, testing requirements, commit conventions, and how releases reach the
Terraform Registry. For build/test/lint/deps commands see
[README → Development](README.md#development); for architecture and
project-specific invariants see [`CLAUDE.md`](CLAUDE.md).

> [!IMPORTANT]
> **Project status & support.** This provider is developed and used in
> production at **Mews**, and maintained in the open under
> [MPL-2.0](LICENSE). We welcome external contributions and will make a
> **best-effort** to triage and review them, but we **cannot commit to any SLA**
> on response or review times, and we may decline changes that don't fit the
> project's direction. Opening an issue to discuss non-trivial work first is the
> best way to avoid wasted effort.

## Before you start

- **Search existing [issues](../../issues) and [pull requests](../../pulls)**
  first to avoid duplicate work.
- **Open an issue to discuss** anything non-trivial (new resource/attribute,
  behavior change, new engine, dependency of note) **before** writing code. Small
  fixes and docs can go straight to a PR.
- By contributing, you agree your changes are licensed under the project's
  [MPL-2.0](LICENSE) license. Every Go file carries the standard header:
  ```go
  // Copyright (c) Mews Systems
  // SPDX-License-Identifier: MPL-2.0
  ```

## Contribution workflow

External contributors don't have push access, so we use the standard
**fork-and-pull** model:

1. **Fork** this repository to your own account.
2. **Clone** your fork and add this repo as `upstream`:
   ```bash
   git clone https://github.com/<you>/terraform-provider-azsqlaccess.git
   cd terraform-provider-azsqlaccess
   git remote add upstream https://github.com/MewsSystems/terraform-provider-azsqlaccess.git
   ```
3. **Branch** off an up-to-date `main`:
   ```bash
   git fetch upstream && git switch -c my-change upstream/main
   ```
4. **Make your change**, following the conventions below. Keep PRs focused — one
   logical change per PR.
5. **Test locally** (see [Testing](#testing)) and run the
   [pre-PR checklist](#before-you-open-a-pr).
6. **Push** to your fork and **open a PR against `upstream/main`**, filling in the
   PR template.
7. **Iterate** on review feedback. We squash-merge, so your PR title must be a
   valid [Conventional Commit](#conventional-commits-required).

## Testing

Both test suites must pass before a change is merged.

### Unit tests (required, no cloud)

Fast, hermetic, no Azure needed. These run automatically on every PR via
`.github/workflows/test.yml` (build, lint, unit tests, and the `generate` diff
check).

```bash
make test            # or: GOTOOLCHAIN=local go test ./...
```

### Acceptance tests (required, real Azure)

> [!WARNING]
> Acceptance tests create **real, billable Azure resources** (Azure SQL and
> PostgreSQL Flexible Server, plus Entra principals) and run real DDL against
> them. **They cost money** and require an Azure subscription you control.

Acceptance tests are **not** run by PR CI — they execute against long-lived
Azure servers and are gated behind `TF_ACC=1`. Run them against **your own**
Azure environment and make sure they're green:

```bash
make testacc         # TF_ACC=1 go test -run '^TestAcc' ./internal/...
```

See [`tests/acceptance/README.md`](tests/acceptance/README.md) for the exact
prerequisites (which Azure resources to deploy, required auth, and the
environment variables to set). A maintainer also runs the **Acceptance Tests**
workflow (`.github/workflows/acceptance.yml`, `workflow_dispatch`) against Mews
infrastructure to verify before merge. Docs- or CI-only PRs (no `.go`, schema,
or `examples/` changes) are exempt from the acceptance run.

## Conventional Commits (required)

We follow the [Conventional Commits v1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)
specification. Because we squash-merge, the **PR title** is the commit that lands
on `main`, and it must conform — it's what drives automated versioning and the
changelog.

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Per the spec, only `feat` and `fix` are required to exist; we use the standard
extended set (the same one `@commitlint/config-conventional` recommends). **No
other types are valid** — `deps:`, `tests:`, `mssql:` etc. are not Conventional
Commits and won't be classified.

| Type | Use for | Appears in changelog |
|---|---|---|
| `feat` | New functionality | **Features** |
| `fix` | Bug fix | **Bug Fixes** |
| `perf` | Performance improvement | **Performance Improvements** |
| `revert` | Reverting a previous commit | **Reverts** |
| `docs` | Docs / examples / `MarkdownDescription` | **Documentation** |
| `refactor` | Behavior-preserving change | **Code Refactoring** |
| `test` | Tests only | hidden |
| `build`, `ci`, `chore`, `style` | Tooling / housekeeping | hidden |

A **scope** is optional and goes in parentheses: `feat(postgres): ...`.
Dependency bumps use the `chore` type with a `deps` scope — `chore(deps): ...`
(this is what Dependabot emits, and it's spec-valid).

A **breaking change** is marked either with `!` after the type/scope
(`feat!:` / `feat(mssql)!:`) **or** a `BREAKING CHANGE:` footer:

```
feat: drop SQL auth support

BREAKING CHANGE: only Entra ID authentication is supported now.
```

The visible/hidden mapping above is configured in
[`release-please-config.json`](release-please-config.json).

### How the version is chosen (pre-1.0)

Until we tag `v1.0.0` the breaking-change budget is intentionally wide open, so
breaking changes bump the **minor**, not the major (`bump-minor-pre-major`):

| Commit | Bump | Example |
|---|---|---|
| `fix:` | patch | `0.1.0` → `0.1.1` |
| `feat:` | minor | `0.1.0` → `0.2.0` |
| `feat!:` / `BREAKING CHANGE:` | minor | `0.1.0` → `0.2.0` |

These rules live in [`release-please-config.json`](release-please-config.json).

## Opening the PR

- **Target `upstream/main`** from your fork's branch, and fill in the PR
  template (including the rollback and security-controls sections).
- **Keep the public surface clean.** This provider publishes to the public
  Terraform Registry, so **never** put real UPNs, names, group/identity names,
  hostnames, database names, or tenant/subscription/object UUIDs anywhere — code,
  comments, `MarkdownDescription`, examples, tests, or docs. Use the placeholder
  table in
  [`CLAUDE.md`](CLAUDE.md#no-personal-or-company-specific-identifiers-anywhere).
- **Regenerate docs** if you touched schema `MarkdownDescription` or anything
  under `examples/`: run `make generate` and **commit the regenerated `docs/`**.
  CI runs `make generate` then `git diff --exit-code` and fails on drift. Never
  hand-edit `docs/`.

### Before you open a PR

```bash
GOTOOLCHAIN=local go build ./...   # CI uses GOTOOLCHAIN=local; reproduce it
GOTOOLCHAIN=local go test ./...    # unit tests
make fmt && make lint
make generate                      # commit any docs/ changes
make testacc                       # acceptance tests (real Azure — see Testing)
```

`GOTOOLCHAIN=local` matches CI exactly (it pins the `go.mod` toolchain and
refuses to auto-fetch a newer one), so build/test errors surface locally instead
of in CI. See [`CLAUDE.md`](CLAUDE.md#stay-consistent-with-cicd) for the Go
directive-coupling traps.

## Review & merge

- A maintainer reviews for correctness, scope, public-surface cleanliness, and
  test coverage, and verifies acceptance tests are green.
- Once approved and all checks pass, a maintainer **squash-merges** using a
  Conventional-Commit title. The merge is the only thing that lands on `main`.
- Per the disclaimer above, review is best-effort with no guaranteed timeline.

## Releasing (maintainers)

Releases are automated with
[release-please](https://github.com/googleapis/release-please); publishing the
signed artifacts is a separate, deliberate step. **You never tag by hand.**

1. Merged Conventional-Commit PRs accumulate on `main`.
2. **release-please** (`.github/workflows/release-please.yml`) maintains an open
   **release PR** with the next computed version + generated `CHANGELOG.md`.
3. **Merging the release PR** tags `vX.Y.Z` and creates the GitHub Release — but
   does **not** publish artifacts.
4. **Publish:** dispatch the **Release** workflow
   (`.github/workflows/release.yml`, `workflow_dispatch`) against the `vX.Y.Z`
   tag. goreleaser cross-compiles, checksums, and **GPG-signs** the artifacts and
   attaches them to the Release. (Requires the `GPG_PRIVATE_KEY` / `PASSPHRASE`
   secrets.)
5. The **Terraform Registry** polls the Release and indexes the new version
   within minutes.

The first release version is pinned to `0.0.1` via `initial-version` in
[`release-please-config.json`](release-please-config.json); after that, versions
are computed from Conventional Commits.

> [!CAUTION]
> Registry indexers cache by commit SHA. **Never** force-push a tag or amend a
> published release — fix forward with a new version instead.
