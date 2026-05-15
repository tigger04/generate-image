Bug-fix issue. References [AC12.5](https://github.com/tadg-paul/pix/issues/12) -- no new AC table is created.

## Bug

`interactive.model-picker.preselect` is documented as "matches an `endpoint_id`" but the implementation uses Go string equality (`m.EndpointID == preselect`). When a user sets:

```yaml
interactive:
  model-picker:
    preselect: xai/grok-imagine-image
```

and FAL returns the endpoint under a longer path -- `xai/grok-imagine-image/edit` for image-to-image -- the exact-equality comparison fails silently and the candidate list is presented in default order.

Reproduction:
- Set `interactive.model-picker.preselect: xai/grok-imagine-image` and `model-picker.always: true`.
- Run `pix gen ref.jpg out.png` (forces `image-to-image` category).
- FAL returns `xai/grok-imagine-image/edit`. The picker opens with default order; the configured model is not on top.

## Affected AC

[AC12.5](https://github.com/tadg-paul/pix/issues/12) -- "When `interactive.model-picker.preselect` matches an `endpoint_id` returned by the `/v1/models` lookup for the active category, that endpoint_id appears as the first line of the candidate list given to the picker." The implementation chose strict equality; "matches" naturally permits a broader interpretation.

## Solution

Treat the `preselect` value as a **Go regular expression** (RE2 syntax). The first endpoint in the result whose `endpoint_id` is matched by the regex moves to the front of the candidate list. This is a power-user feature; the install base is primarily one user who wants the flexibility.

Behaviour:

- Empty `preselect` -> no-op (unchanged).
- Valid regex matches at least one `endpoint_id` -> first matching entry moves to the front. Order of evaluation follows the slice's existing order (FAL response order, which we don't sort).
- Valid regex matches no `endpoint_id` -> no-op (no error). Default order presented.
- Invalid regex (compile error) -> log a one-line warning to stderr naming the bad regex and the compile error, then proceed with default order. The picker still opens; the user is not blocked by a typo in their config.

Implementation in `models.go::reorderPreselect`:

```go
re, err := regexp.Compile(preselect)
if err != nil {
    fmt.Fprintf(os.Stderr, "Warning: model-picker.preselect %q is not a valid regex: %v (proceeding without preselect)\n", preselect, err)
    return models
}
for i, m := range models {
    if re.MatchString(m.EndpointID) {
        // partition: move models[i] to front
        ...
    }
}
```

The existing exact-match assertion in RT-12.9 (`fal-ai/middle` against `[..., fal-ai/middle, ...]`) is satisfied by regex semantics too -- a literal string is its own regex that matches itself. So no existing test breaks.

### Why regex over substring

Tadg's call. Substring would solve the immediate `xai/grok-imagine-image` -> `xai/grok-imagine-image/edit` case, but regex gives anchored matches (`^xai/grok-imagine-image$` for exact, `/edit$` for any edit variant) and alternation (`grok|flux`) for users juggling multiple habitual defaults. The cost is one new error path (invalid regex) handled by a single stderr line.

## Tests

- **RT-12.9** (existing) -- preselect `fal-ai/middle` against `[fal-ai/aaa, fal-ai/middle, fal-ai/zzz]`. Literal regex matches the exact entry. Passes unchanged.
- **RT-12.10** (existing) -- preselect `fal-ai/not-in-list` against `[fal-ai/aaa, fal-ai/bbb, fal-ai/ccc]`. No match. Default order. Passes unchanged.
- **RT-12.11** (existing) -- empty preselect. Early return. Passes unchanged.
- **RT-14.1** (new) -- preselect `xai/grok-imagine-image` against `[fal-ai/aaa, xai/grok-imagine-image/edit, fal-ai/zzz]`. Regex (treated as substring-equivalent literal pattern) matches the long-form endpoint; that entry moves to the front.
- **RT-14.2** (new) -- preselect `^xai/.*` against `[fal-ai/aaa, xai/grok-imagine-image/edit, xai/other]`. Anchored regex matches the first `xai/...` entry; that entry moves to the front.
- **RT-14.3** (new) -- preselect `[invalid` (unmatched `[`). Compile fails; pix logs a warning to stderr and proceeds with default order; the picker still opens; the first candidate is whatever was first in the FAL response.

`PROCEED 14` already received -- implementation to follow.
