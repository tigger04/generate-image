# generate-image

A minimal CLI tool that generates images from text prompts via the [FAL API](https://fal.ai). Pipe a prompt in, get an image out.

<img width="100%" alt="Terminal example" src="https://github.com/user-attachments/assets/de7740f5-4735-47d2-a943-e481b3b9c343" />

## Quickstart

### Prerequisites

- Go 1.22+
- A [FAL API key](https://fal.ai/dashboard/keys)
- ImageMagick (`magick`) -- optional, for format conversion

### Install

```bash
git clone https://github.com/tigger04/generate-image.git
cd generate-image
make install
```

This compiles the binary to `~/.local/bin/generate-image` and copies `config.yaml` to `~/.config/generate-image/`. On first install, a template `.env` is created -- edit it to add the FAL API key:

```bash
vi ~/.config/generate-image/.env
```

Alternatively, set the `FAL_KEY` environment variable or configure a key command in `config.yaml` (see [Configuration](#configuration)).

### Usage

```bash
> echo "a red cat sitting on a wall" | generate-image cat
Cost: $0.02 (unit: images) for model xai/grok-imagine-image (source: FAL API)
Wrote cat.jpg

> echo "a blueprint" | generate-image blueprint.png
# API returns JPEG, converted to PNG via magick:
Cost: $0.02 (unit: images) for model xai/grok-imagine-image (source: FAL API)
Wrote blueprint.png (converted jpg to png)

> echo "test prompt" | generate-image --dry-run test
POST https://fal.run/xai/grok-imagine-image
{
  "prompt": "test prompt"
}
Output: test
(dry run -- no API call made)

> echo "A spoon eating a man wearing a hat" | generate-image -q -p landscape
# generates quietly, opens in default viewer (or preview-command from config)
```

### Flags

| Flag | Description |
|------|-------------|
| `-h`, `--help` | Show usage |
| `--version` | Show version |
| `-q`, `--quiet` | Suppress cost output |
| `--dry-run` | Print what would happen without calling the API |
| `-p`, `--preview` | Open the image after generation |

## Configuration

Configuration lives at `~/.config/generate-image/config.yaml` (or next to the binary during development).

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

## Project files

| File | Purpose |
|------|---------|
| `main.go` | Single-file CLI entry point |
| `config.yaml` | Default model configuration |
| `.env.example` | API key template |
| `Makefile` | Build, test, install, lint targets |
| `tests/regression/` | Regression test suite (30 tests) |
| `tests/one_off/` | One-off tests |
| `docs/vision.md` | Project vision and roadmap |

## Development

```bash
make build          # Compile binary
make test           # Lint + run regression tests
make test-one-off   # Run one-off tests
```

All regression tests use local HTTP test servers -- no real API calls, no API key needed.

## Licence

MIT -- Copyright Taḋg Paul
