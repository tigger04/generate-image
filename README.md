# pix

A minimal CLI for generating and pricing images via the [FAL API](https://fal.ai). Pipe a prompt in, get an image out.

<img width="100%" alt="Terminal example" src="https://github.com/user-attachments/assets/de7740f5-4735-47d2-a943-e481b3b9c343" />

## Quickstart

### Prerequisites

- Go 1.22+
- A [FAL API key](https://fal.ai/dashboard/keys)
- ImageMagick (`magick`) -- optional, for format conversion

### Install

```bash
git clone https://github.com/tadg-paul/pix.git
cd pix
make install
```

This compiles the binary to `~/.local/bin/pix` and creates `~/.config/pix/config.yaml` from the template. Edit it to configure the API key and model -- see [Configuration](#configuration).

### Usage

Create a new image from a prompt:

```bash
> echo "a red cat sitting on a wall" | pix generate-image cat
Cost: $0.02 (unit: images) for model xai/grok-imagine-image (source: FAL API)
Wrote cat.jpg

> echo "a blueprint" | pix generate-image blueprint.png
# API returns JPEG, converted to PNG via magick:
Cost: $0.02 (unit: images) for model xai/grok-imagine-image (source: FAL API)
Wrote blueprint.png (converted jpg to png)

> echo "test prompt" | pix generate-image --dry-run test
POST https://fal.run/xai/grok-imagine-image
{
  "prompt": "test prompt"
}
Output: test
(dry run -- no API call made)

> echo "A spoon eating a man wearing a hat" | pix --quiet generate-image -p landscape
# generates quietly, opens in default viewer (or preview-command from config)
```

Edit an existing image (or merge several) with `edit-image`:

```bash
> echo "make it dramatic" | pix edit-image photo.jpg dramatic.jpg
⚠️  Using photo.jpg as reference image (will be sent to FAL)
Cost: $0.02 (unit: images) for model xai/grok-imagine-image (source: FAL API)
Wrote dramatic.jpg

> echo "merge these" | pix edit-image a.jpg b.jpg merged.jpg
⚠️  Using a.jpg as reference image (will be sent to FAL)
⚠️  Using b.jpg as reference image (will be sent to FAL)
Wrote merged.jpg

> pix edit-image out.png
Error: edit-image requires at least one reference image
       (the last positional is the output file; earlier positionals are reference images)
```

Look up cost without generating:

```bash
> pix cost
Model: xai/grok-imagine-image
Unit price: $0.02 per images (source: FAL API)
Estimated cost: $0.0200 per call based on usage history (source: FAL API)
```

> **Note:** `generate-image` will *also* accept reference images if earlier positionals exist as image files -- those positionals are sent to the FAL edit endpoint just as `edit-image` does. The two subcommands share the same pipeline; `edit-image` simply enforces that at least one reference is supplied.

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `generate-image <output>` | Generate a new image from a prompt on stdin. Will also accept up to 3 reference images as earlier positionals if those files exist (in which case it edits rather than generates). Alias: `gen-img`. |
| `edit-image <refs...> <output>` | Edit one or more existing images using a prompt on stdin. At least one reference image is required (max 3). The last positional is the target. |
| `cost` | Query pricing for the configured model (no generation) |

Run `pix <subcommand> --help` for subcommand-specific usage.

### Flags

**Global flags** (placed before the subcommand):

| Flag | Description |
|------|-------------|
| `-h`, `--help` | Show usage |
| `--version` | Show version |
| `-q`, `--quiet` | Suppress non-error output |

**Subcommand flags** (placed after the subcommand):

| Flag | Subcommand | Description |
|------|------------|-------------|
| `--dry-run` | generate-image, edit-image, cost | Show what would happen without calling the API |
| `-p`, `--preview` | generate-image, edit-image | Open the image after generation |

`--help` is mutually exclusive with all other flags and arguments.

## Configuration

Configuration lives at `~/.config/pix/config.yaml` (or next to the binary during development).

```yaml
# Model to use for image generation
model: xai/grok-imagine-image

# API key resolution (optional -- falls back to FAL_KEY env var or .env file)
api-keys:
  fal:
    # Run a command to retrieve the key (e.g. password manager)
    command: op read op://vault/fal/credential
    # Or read from a file
    # file: /path/to/fal.key

# Custom preview command (optional -- defaults to open/xdg-open/start)
# preview-command: chafa
```

### API key resolution priority

1. `FAL_KEY` environment variable
2. `api-keys.fal.command` in config (stdout is the key)
3. `api-keys.fal.file` in config (file contents are the key)
4. `.env` file in the config directory (`FAL_KEY=...`)

## Extension handling

If no file extension is provided, the API response format is used (typically `.jpg`). If the requested extension differs from the API format, ImageMagick (`magick`) converts automatically. If `magick` is not available, the tool exits with an error.

## Documentation

| Document | Description |
|----------|-------------|
| [Vision](docs/vision.md) | Project goals, how it works, technology choices |
| [Architecture](docs/architecture.md) | Component overview, design decisions, [roadmap](docs/architecture.md#roadmap) |

## Project files

| File | Purpose |
|------|---------|
| `main.go` | CLI entry point and subcommand dispatch |
| `genimg.go` | gen-img subcommand handler |
| `cost.go` | cost subcommand handler |
| `config.go` | Config loading and API key resolution |
| `fal.go` | FAL API HTTP client helpers |
| `config.yaml` | Default model configuration |
| `Makefile` | Build, test, install, lint targets |
| `tests/regression/` | Regression test suite (54 tests) |
| `tests/one_off/` | One-off tests |

## Development

```bash
make build          # Compile binary
make test           # Lint + run regression tests
make test-one-off   # Run one-off tests
```

All regression tests use local HTTP test servers -- no real API calls, no API key needed.

## Licence

MIT -- Copyright Tadg Paul
