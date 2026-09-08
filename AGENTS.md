## Project

**spin**

`spin` is a language-agnostic scaffolder for external templates. A template is any directory with a `spin.toml` manifest and a `_base/` tree of files. `spin new <name> --template <spec>` resolves the template, prompts for params (or accepts them via `--param`), renders `_base/` into a new project, runs `[[pre]]` and `[[post]]` hooks, and removes the output `spin.toml`.

`spin` does not care what language, framework, or build tool a template produces. It only owns the load / prompt / render / hook pipeline.

### Commands

| Command                          | Purpose                                       |
| -------------------------------- | --------------------------------------------- |
| `spin new [<name>] [<template>]` | Scaffold a project from a template            |
| `spin add <spec>`                | Pin a template locally for offline use        |
| `spin list`                      | Show pinned templates                         |
| `spin update [name]`             | Refresh a pinned template's cache             |
| `spin remove <name>`             | Remove a pin (`--purge` to delete cache)      |
| `spin search <query>`            | Search registered local registries            |
| `spin registry ...`              | Add / list / update / remove local registries |
| `spin init <name>`               | Scaffold a new template directory             |
| `spin version`                   | Print the version                             |

### Template specs

`--template` accepts:

- Local path: `~/templates/go-cli`, `./go-cli`
- Git URL: `https://github.com/example/go-cli-template.git`
- GitHub shorthand: `example/go-cli-template` (expanded to the HTTPS git URL)
- Pinned name: `go-cli-template`
- Registry shorthand: `official/go-cli` (resolved from a registered registry)

### Constraints

- **Language-agnostic**: templates may target any stack.
- **External templates only**: no embedded defaults.
- **Single static binary**: `CGO_ENABLED=0`, no plugins, no daemon.
- **Two pipelines**: template rendering (`spin new`) and registry/pin management (`spin add`, `spin list`, `spin update`, `spin remove`, `spin search`) share the `Template` type but are otherwise independent.
- **Local-first registries**: `spin search` reads locally registered registries; there is no public registry server in this repo.

## Technology Stack

| Layer               | Choice                             |
| ------------------- | ---------------------------------- |
| Language            | Go 1.27                            |
| CLI framework       | Cobra + Fang (styled help/version) |
| Interactive prompts | Huh v2                             |
| Terminal styling    | Lip Gloss v2                       |
| TOML parsing        | BurntSushi/toml                    |
| Template engine     | Go `text/template`                 |

## Development

```sh
task build              # ./bin/spin
task test               # go test ./... -count=1
task lint               # golangci-lint run ./... (if installed)
task fmt                # go fmt ./... && golangci-lint fmt ./...
```

Or without Task:

```sh
go fmt ./...
golangci-lint fmt ./...
go build -o bin/spin .
go test ./... -count=1
go vet ./...
golangci-lint run ./...
```

## Conventions

- Keep `spin` itself small and language-agnostic.
- Templates are the only extension surface.
- All file-changing work goes through the normal tool workflow; no special GSD command is required for this repo.
- **Error messages use zero-level nesting**: no `registry:`, `spin <cmd>:`, or other category prefixes. Errors are plain, direct sentences aimed at the end user (e.g. `"spin.toml not found in /path"`, not `"registry: add local: spin.toml not found in /path"`). Internal sentinel errors (used with `errors.Is`) may optionally carry a short prefix for disambiguation in tests.

## Architecture

- `cmd/` — Cobra commands and CLI wiring.
- `internal/template/` — template loading, `spin.toml` parsing, rendering, hooks.
- `internal/params/` — param types and Huh form generation.
- `internal/registry/` — pinned templates (`pinned.json`) and local registry management (`registries.json`).
- `internal/version/` — build version.
