<!-- Version: 0.2 | Last updated: 2026-05-03 -->

# Vision

## The Holy Grail

A useful cli and maybe gui to let you create and edit images, using generateive AI models, that will run locally on your device. Dream on.

## And back down to earth ...

`generate-image` is a minimal CLI tool that generates images from text prompts via the [FAL API](https://fal.ai). It reads a prompt from stdin, sends it to a configurable model, and writes the resulting image to disk.

```bash
echo "a sunset over Dublin Bay" | generate-image sunset
cat description.txt | generate-image --preview poster
```

The first argument is the output filename. An extension is optional -- if omitted, the API response format is used.

## Goals

- **Single responsibility:** one prompt in, one image out.
- **Zero friction:** `make install` places the binary on `PATH`; a YAML config file is the only setup.
- **Pipeline-friendly:** reads stdin, writes a file, reports status to stderr. No interactive prompts.
- **Cost-aware:** reports generation cost to stderr when the FAL pricing API has data.

## Non-goals (for now)

- GUI or web interface.
- Batch generation or multi-image output.
- Video generation.
- Provider abstraction (FAL is the only backend).

## How it works

1. The tool takes an output filename as its argument. Flags (`--quiet`, `--dry-run`, `--preview`) are optional.
2. It reads the text prompt from stdin.
3. It resolves the FAL API key via a priority chain: `FAL_KEY` env var, config command, config file, `.env` fallback.
4. It reads the model from `config.yaml`.
5. It calls the FAL API with the configured model and prompt.
6. It downloads the resulting image and writes it to the output path, handling extension detection and format conversion via ImageMagick if needed.
7. It reports cost to stderr (unless `--quiet`).
8. If `--preview` is specified, it opens the image in the configured viewer.

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

The config file lives at `~/.config/generate-image/config.yaml`, with fallback to the binary directory for development.

### API key resolution priority

1. `FAL_KEY` environment variable
2. `api-keys.fal.command` in config (runs a command, uses stdout)
3. `api-keys.fal.file` in config (reads a file)
4. `.env` file in the config directory (legacy fallback)

## Technology

- **Language:** Go 1.22+
- **Dependencies:** `gopkg.in/yaml.v3` (config parsing). Standard library for everything else.
- **Optional:** ImageMagick (`magick`) for format conversion
- **Installation:** `make install` copies the binary to `~/.local/bin/generate-image`

## Licence

MIT -- Copyright Tadg Paul
