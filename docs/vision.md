<!-- Version: 0.1 | Last updated: 2026-05-02 -->

# Vision

## Overview

`generate-image` is a minimal CLI tool that generates images from text prompts via the [FAL API](https://fal.ai). It reads a prompt from stdin, sends it to a configurable model, and writes the resulting image to disk.

```bash
cat description.txt | generate-image sunset.png
echo "a sunset over Dublin Bay" | generate-image sunset.png
```

The first argument is the output filename. It is required.

## Goals

- **Single responsibility:** one prompt in, one image out.
- **Zero friction:** `make install` places the executable on `PATH`; a `.env` file and a small YAML config are the only setup.
- **Pipeline-friendly:** reads stdin, writes a file, prints the output path to stdout. No interactive prompts.
- **Cost-aware:** reports generation cost to stderr when the FAL API provides pricing information.

## Non-goals (for now)

- GUI or web interface.
- Batch generation or multi-image output.
- Image-to-image, inpainting, or editing workflows.
- Video generation.
- Provider abstraction (FAL is the only backend).

## How It Works

1. The tool requires one argument: the output filename. It fails if none is provided.
2. It reads the text prompt from stdin.
3. It loads the FAL API key from `.env` in the tool's install directory.
4. It reads the model from `config.yaml` in the tool's install directory.
5. It calls the FAL API with the configured model and prompt.
6. It downloads the resulting image and writes it to the output path.
7. If the FAL API returns cost/pricing data, it reports the cost to stderr.

## Configuration

### Environment (`.env`)

```
FAL_KEY=your-fal-api-key
```

The `.env` file lives alongside the script, not in the working directory. This allows the tool to be invoked from any directory without requiring a per-project `.env`.

### Model configuration (`config.yaml`)

```yaml
model: fal-ai/grok-2-aurora
```

A single YAML file with one key for now. The default model is Grok. This file lives alongside the script and `.env`.

## Technology

- **Language:** Python 3.12+
- **FAL SDK:** `fal-client` (same library used by `storyboard-gen`)
- **Config loading:** `python-dotenv` for `.env`, `PyYAML` for `config.yaml`
- **Installation:** `make install` symlinks the entry point to `~/.local/bin/generate-image`

## Roadmap

Future extensions, in rough priority order. None of these are in scope for the initial release.

| Feature | Description |
|---------|-------------|
| Output format selection | Support `--format png\|jpg\|webp` |
| Image dimensions | Support `--width` and `--height` or `--size` presets |
| Model override | `--model` flag to override `config.yaml` on a per-call basis |
| Multiple models in config | Named model profiles in `config.yaml` |
| Batch mode | Accept multiple prompts (one per line) and generate images in parallel |
| Cost tracking | Cumulative cost log for budgeting |
| Prompt templates | Reusable prompt fragments or prefix/suffix in config |
| Provider abstraction | Support additional image generation APIs beyond FAL |

## Licence

MIT -- Copyright Tadg Paul
