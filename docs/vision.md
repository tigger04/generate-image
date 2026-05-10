<!-- Version: 0.3 | Last updated: 2026-05-10 -->

# Vision

## The Holy Grail

A useful CLI (and maybe GUI) to let you create and edit images using generative AI models, running locally on your device. Dream on.

## And back down to earth ...

`pix` is a minimal CLI tool that interacts with the [FAL API](https://fal.ai) for image-related operations. It is built around subcommands so that distinct operations remain discoverable and individually testable as the tool grows.

```bash
echo "a sunset over Dublin Bay" | pix gen-img sunset
cat description.txt | pix gen-img --preview poster
pix cost
```

## Goals

- **Subcommand-based:** every distinct operation is its own subcommand. New features extend the surface, they don't bloat existing commands.
- **Zero friction:** `make install` places the binary on `PATH`; a YAML config file is the only setup.
- **Pipeline-friendly:** reads stdin, writes files, reports status to stderr. No interactive prompts.
- **Cost-aware:** reports generation cost when the FAL pricing API has data; standalone cost lookup via `pix cost`.

## Non-goals (for now)

- GUI or web interface.
- Batch generation or multi-image output.
- Video generation.
- Provider abstraction (FAL is the only backend).

## Subcommands

### `pix gen-img <output>`

Reads a text prompt from stdin and generates an image via the FAL API. Writes the result to `<output>` (extension optional -- if omitted, the API response format is used).

### `pix cost`

Queries pricing for the configured model without generating an image. Reports both the unit price and a historical estimate based on past usage.

## Flag system

**Global flags** (placed before the subcommand): `--quiet` / `-q`, `--version`, `--help` / `-h` (top-level).

**Subcommand flags** (placed after the subcommand): `--dry-run`, `--preview` / `-p` (gen-img only), `--help` / `-h` (subcommand-specific).

`--help` is mutually exclusive with all other flags and arguments.

## Configuration

### `config.yaml`

```yaml
model: xai/grok-imagine-image

api-keys:
  fal:
    command: op read op://vault/fal/credential
    # file: /path/to/fal.key

preview-command: chafa
```

The config file lives at `~/.config/pix/config.yaml`, with fallback to the binary directory for development.

### API key resolution priority

1. `FAL_KEY` environment variable
2. `api-keys.fal.command` in config (runs a command, uses stdout)
3. `api-keys.fal.file` in config (reads a file)
4. `.env` file in the config directory (legacy fallback)

## Technology

- **Language:** Go 1.22+
- **Dependencies:** `gopkg.in/yaml.v3` (config parsing). Standard library for everything else.
- **Optional:** ImageMagick (`magick`) for format conversion
- **Installation:** `make install` copies the binary to `~/.local/bin/pix`

## Licence

MIT -- Copyright Tadg Paul
