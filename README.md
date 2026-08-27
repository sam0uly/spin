# Spin

[![Go Version](https://img.shields.io/github/go-mod/go-version/sam0uly/spin?style=flat-square&color=ff5a1f&labelColor=1c1b19)](https://go.dev/)
[![Go Report Card](https://img.shields.io/badge/go%20report-A%2B-ff5a1f?style=flat-square&labelColor=1c1b19)](https://goreportcard.com/report/github.com/sam0uly/spin)
[![Go Reference](https://img.shields.io/badge/pkg.go.dev-reference-ff7a45?style=flat-square&labelColor=1c1b19)](https://pkg.go.dev/github.com/sam0uly/spin)
[![License](https://img.shields.io/github/license/sam0uly/spin?style=flat-square&color=e84a0a&labelColor=1c1b19)](LICENSE)
[![Release](https://img.shields.io/github/v/release/sam0uly/spin?include_prereleases&style=flat-square&color=ff5a1f&labelColor=1c1b19&label=release)](https://github.com/sam0uly/spin/releases)

A project scaffolding CLI for any language, framework, or stack, enabling developers to bootstrap fully configured projects from reusable templates sourced from GitHub, local directories, or template registries.

```bash
spin new myapp https://github.com/user/spin-template.git
```

## Install

```bash
go install github.com/sam0uly/spin@latest
curl -sSfL https://spincli.pages.dev/install.sh | sh
```

Single static binary. Needs git on $PATH.

## Getting started

The documentation: [Spin docs](https://spin.samouly.fun)
Go pkg: https://pkg.go.dev/github.com/sam0uly/spin (just have the readme so its not for learning)

## Commands

| Command                          | Description                            |
| -------------------------------- | -------------------------------------- |
| `spin new [name] [template]`     | Scaffold a project from a template     |
| `spin add [spec]`                | Pin a template locally for offline use |
| `spin list`                      | Show pinned templates                  |
| `spin update [name]`             | Refresh a pin's cache                  |
| `spin remove [name]`             | Remove a pin (--purge deletes cache)   |
| `spin search [query]`            | Search registered registries           |
| `spin registry add [name] [url]` | Register a registry                    |
| `spin registry list`             | Show registries                        |
| `spin init [name]`               | Scaffold a new template directory      |

## Template Specs

```txt
./templates/go-cli              local path
https://github.com/me/repo.git  git URL
go-cli-template                 pinned name
official/go-cli                 registry shorthand
```

## Template Anatomy

```txt
my-template/
  spin.toml      params, hooks, include/exclude rules
  _base/         directory tree rendered into the project
  _pre/          scripts run before rendering
  _post/         scripts run after rendering
```

## License

Spin is licensed under [Apache 2.0](./LICENSE)
