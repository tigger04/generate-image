<!-- Version: 0.2 | Last updated: 2026-05-10 -->

# Architecture

## Overview

`pix` is a single-binary CLI tool written in Go. It has no runtime dependencies beyond the binary itself (ImageMagick is optional for format conversion). The architecture is deliberately minimal -- a handful of files in package `main`, no subdirectories, no abstractions beyond what subcommand dispatch requires.

## Components

```
                    ┌────────────────┐
stdin (prompt)─────►│                │─► FAL API ─► image file
                    │   pix binary   │─► FAL pricing API ─► cost to stderr
config.yaml ───────►│                │─► preview command ─► image viewer
                    └────────────────┘
                          │
                    ┌─────┴─────┐
                    │           │
                gen-img       cost
```

### File layout

All code lives in package `main` at the project root.

| File | Purpose |
|------|---------|
| `main.go` | Entry point, global flag parsing, subcommand dispatch |
| `genimg.go` | `pix gen-img` handler -- generates images from prompts |
| `cost.go` | `pix cost` handler -- queries pricing without generation |
| `config.go` | `config.yaml` loading, API key resolution, config directory resolution |
| `fal.go` | FAL API HTTP helpers (generation, pricing, historical estimate) |

### Subcommands

| Subcommand | Purpose |
|------------|---------|
| `gen-img <output>` | Reads prompt from stdin, writes image to disk |
| `cost` | Queries pricing for configured model |

Each subcommand parses its own flags, including a subcommand-specific `--help` / `-h` and `--dry-run` (where applicable).

### Flag system

Two categories enforced strictly:

- **Global flags** (must be placed before the subcommand): `--quiet` / `-q`, `--version`, top-level `--help` / `-h`
- **Subcommand flags** (must be placed after the subcommand): `--dry-run`, `--preview` / `-p`, subcommand-specific `--help` / `-h`

Misplaced flags exit non-zero with a clear error. `--help` is mutually exclusive with all other flags and arguments.

### Configuration

Two files in the config directory (`~/.config/pix/` or next to the binary):

- **`config.yaml`** -- model, API key sources, preview command
- **`.env`** -- fallback API key storage (optional, legacy)

The config directory is resolved at runtime by checking for `config.yaml` or `.env` next to the binary first, then falling back to the XDG location. This allows development (config next to binary) and installed (XDG) use without any flag or env var.

### API integration

The tool calls three FAL endpoints:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `https://fal.run/{model}` | POST | Image generation (`gen-img`) |
| `https://api.fal.ai/v1/models/pricing?endpoint_id={model}` | GET | Unit price lookup (`cost`, post-generation cost) |
| `https://api.fal.ai/v1/models/pricing/estimate` | POST | Historical cost estimate (`cost`) |

All use `Authorization: Key {fal_key}` headers. The `FAL_BASE_URL` environment variable redirects all endpoints to a test server via `httptest.NewServer`.

### Testing

All 54 regression tests run the compiled binary as a subprocess via `os/exec`. The FAL API is intercepted using Go's `httptest.NewServer` -- no real API calls are made during `make test`. The Makefile sets `HOME` to a temp directory to prevent personal config from leaking into tests.

## Design decisions

| Decision | Rationale |
|----------|-----------|
| Subcommand structure (vs single-purpose binaries) | Discoverable surface, single config, single install. Future operations land cleanly. |
| Multi-file package main (vs monolithic main.go) | Each subcommand and concern in its own file -- navigability without architectural overhead. |
| Strict flag positioning | Removes ambiguity. Misplaced flags fail loudly; users learn the rule once. |
| Go, not Python | Single static binary. No venv, no pip, no runtime. Trivial cross-compilation. |
| No FAL SDK | The FAL API is a handful of HTTP calls. A dependency is not justified. |
| `sh -c` for user commands | Config commands (key retrieval, preview) are user-specified shell expressions. |
| Extension from Content-Type | The FAL API returns JPEG by default regardless of what the user requests. Detecting and handling this is better than surprising the user with a misnamed file. |

## Roadmap

Future enhancements, in rough priority order:

| Feature | Description | Complexity |
|---------|-------------|------------|
| Reference image / edit mode | Add reference image support to `pix gen-img` via positional args. Uses FAL's `/edit` endpoint with `image_urls` parameter. See [#4](https://github.com/tadg-paul/pix/issues/4). | Medium |
| `--model` flag | Override `config.yaml` model per invocation. Enables comparing models. | Small |
| Image dimensions | Support `--aspect-ratio` or `--size` presets. FAL API accepts `aspect_ratio` ("1:1", "16:9") and `resolution` ("1k", "2k"). | Small |
| Homebrew formula | Cross-compiled binaries for Darwin/Linux/Windows. `make release` with GitHub releases. See [#3](https://github.com/tadg-paul/pix/issues/3). | Medium |
| Batch mode | Accept multiple prompts (one per line), generate in parallel. | Medium |
| Cost tracking | Cumulative cost log for budgeting across sessions. | Medium |
| Prompt templates | Reusable prefix/suffix fragments in config. | Small |
| Provider abstraction | Support APIs beyond FAL (e.g. Replicate, direct vendor APIs). Not planned until a concrete need arises. | Large |
