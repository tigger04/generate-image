// ABOUTME: Per-model-family quirks (declarative). Currently captures the
// ABOUTME: image_url vs image_urls difference; will grow as more quirks land.

package main

import (
	"fmt"
	"os"
	"strings"
)

// modelHandler captures the declarative quirks for a family of FAL models.
//
// Ported in spirit from storyboard-gen's EditHandler. Storyboard-gen has many
// more fields (sizing strategy, safety defaults, edit_accepts_sizing,
// prompt-rewriting, etc.) -- pix is image-only and simpler, so we keep only
// what we currently differentiate on. New fields land as new quirks emerge.
type modelHandler struct {
	// Patterns: substrings (case-insensitive) matched against the model id.
	// First handler in the registry whose Patterns matches the model wins.
	Patterns []string

	// RefField: the JSON key under which reference image URIs are sent to FAL.
	// "image_url" (singular) sends the first ref as a string value.
	// "image_urls" (plural, default) sends all refs as an array.
	RefField string

	// SafetyDefaults: keys/values merged into every request payload for this
	// family. Pix is for private use; we default to safety-off wherever the
	// model offers a knob, to avoid spurious rejections (e.g. nano-banana
	// flagging an aerial sketch because it contains a school). Per-family
	// values mirror storyboard-gen's StillHandler safety_defaults.
	SafetyDefaults map[string]interface{}
}

// modelHandlers is the dispatch table. Order matters: more-specific patterns
// first; the final entry (empty patterns) is the default that always matches.
var modelHandlers = []modelHandler{
	// Kontext family: singular image_url, safety_tolerance maxed (storyboard-gen
	// uses "6"; FAL's documented max).
	{
		Patterns:       []string{"kontext"},
		RefField:       "image_url",
		SafetyDefaults: map[string]interface{}{"safety_tolerance": "6"},
	},

	// Reve family: singular image_url. No documented safety knob.
	{
		Patterns: []string{"reve"},
		RefField: "image_url",
	},

	// Emu 3.5 image: singular image_url. No documented safety knob.
	{
		Patterns: []string{"emu-3.5"},
		RefField: "image_url",
	},

	// Flux 2 family (incl. pro / max): safety checker off.
	{
		Patterns:       []string{"flux-2"},
		RefField:       "image_urls",
		SafetyDefaults: map[string]interface{}{"enable_safety_checker": false},
	},

	// Seedream (ByteDance): safety checker off.
	{
		Patterns:       []string{"seedream"},
		RefField:       "image_urls",
		SafetyDefaults: map[string]interface{}{"enable_safety_checker": false},
	},

	// Hunyuan Image (Tencent): safety checker off.
	{
		Patterns:       []string{"hunyuan-image"},
		RefField:       "image_urls",
		SafetyDefaults: map[string]interface{}{"enable_safety_checker": false},
	},

	// Recraft: safety checker off.
	{
		Patterns:       []string{"recraft"},
		RefField:       "image_urls",
		SafetyDefaults: map[string]interface{}{"enable_safety_checker": false},
	},

	// Instant Character: safety checker off (per storyboard-gen
	// InstantCharacterHandler.safety_defaults).
	{
		Patterns:       []string{"instant-character"},
		RefField:       "image_url",
		SafetyDefaults: map[string]interface{}{"enable_safety_checker": false},
	},

	// Flux 1.x family (fallback before the catch-all): safety checker off
	// (per storyboard-gen FluxHandler.safety_defaults). Use a narrow pattern
	// so this doesn't shadow more specific entries.
	{
		Patterns:       []string{"flux-general", "flux-pro/v1", "flux/dev"},
		RefField:       "image_urls",
		SafetyDefaults: map[string]interface{}{"enable_safety_checker": false},
	},

	// Default: plural image_urls, no safety knob assumed. Covers grok,
	// glm-image, nano-banana, gpt-image-1.5, firered, qwen, ideogram, and
	// anything new pix hasn't profiled yet. Models with a known but
	// non-standard safety knob can be promoted to their own entry above.
	{
		Patterns: nil, // empty -> always matches; must remain last
		RefField: "image_urls",
	},
}

// handlerFor returns the first modelHandler whose Patterns match the given
// model id. The final entry always matches.
func handlerFor(model string) modelHandler {
	lower := strings.ToLower(model)
	for _, h := range modelHandlers {
		if len(h.Patterns) == 0 {
			return h
		}
		for _, p := range h.Patterns {
			if strings.Contains(lower, p) {
				return h
			}
		}
	}
	// Defensive fallback. The default entry should always match before this.
	return modelHandler{RefField: "image_urls"}
}

// refPayload assembles the reference-image portion of a FAL request payload
// according to the handler's RefField setting. For singular ("image_url"),
// only the first URI is sent; pix warns to stderr when extra refs are
// dropped (matching storyboard-gen's behaviour).
func (h modelHandler) refPayload(uris []string, globalQuiet bool) (string, interface{}) {
	if h.RefField == "image_url" {
		if len(uris) > 1 && !globalQuiet {
			fmt.Fprintf(os.Stderr, "Warning: model accepts a single reference image; using the first of %d (others dropped)\n", len(uris))
		}
		return "image_url", uris[0]
	}
	return "image_urls", uris
}
