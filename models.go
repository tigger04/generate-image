// ABOUTME: Model picker flow for --pick-model.
// ABOUTME: Fetches FAL /v1/models, presents to the configured picker, returns the selected endpoint_id.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type modelEntry struct {
	EndpointID string `json:"endpoint_id"`
	Metadata   struct {
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Category    string   `json:"category"`
		Status      string   `json:"status"`
		Tags        []string `json:"tags"`
		ModelURL    string   `json:"model_url"`
		LicenseType string   `json:"license_type"`
		UpdatedAt   string   `json:"updated_at"`
	} `json:"metadata"`
}

type modelsListResponse struct {
	Models []modelEntry `json:"models"`
}

// runModelPickerFlow fetches FAL models for the given category, presents them
// via the configured picker, and returns the selected endpoint_id.
// hasRefs determines category: text-to-image with zero refs, image-to-image with refs.
func runModelPickerFlow(cfg *config, falKey string, hasRefs bool) (string, bool, error) {
	picker := effectivePicker(cfg)

	fields := strings.Fields(picker)
	if len(fields) == 0 {
		return "", false, fmt.Errorf("picker is empty")
	}
	pickerBin := fields[0]
	expandedBin, err := expandTilde(pickerBin)
	if err != nil {
		return "", false, err
	}
	if _, err := exec.LookPath(expandedBin); err != nil {
		return "", false, fmt.Errorf("picker %q not found on PATH: %w", pickerBin, err)
	}

	category := "text-to-image"
	if hasRefs {
		category = "image-to-image"
	}

	models, err := fetchModels(falKey, category)
	if err != nil {
		return "", false, err
	}
	if len(models) == 0 {
		return "", false, fmt.Errorf("FAL /v1/models returned no models for category=%s", category)
	}

	// Each candidate line is just the endpoint_id -- a clean list, easy to scan
	// and search. Model metadata is written to per-model files in a tempdir, so
	// fzf's preview pane can show prettified details for whichever line the user
	// is currently highlighting (preview command is `cat <tempdir>/{}.md`).
	tempDir, err := os.MkdirTemp("", "pix-model-info-")
	if err != nil {
		return "", false, fmt.Errorf("creating model-info tempdir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Reorder so the preselected endpoint_id (if present) is the first
	// candidate. fzf highlights the first line by default, giving the user a
	// one-keystroke confirm. If preselect is empty or doesn't match any model,
	// the candidate order is the default (API order).
	models = reorderPreselect(models, cfg.Interactive.ModelPicker.Preselect)

	candidates := make([]string, 0, len(models))
	for _, m := range models {
		candidates = append(candidates, m.EndpointID)
		if err := writeModelDetails(tempDir, m); err != nil {
			return "", false, fmt.Errorf("writing model details: %w", err)
		}
	}

	headerArg := "--header='Select a FAL model (" + category + ")'"
	previewArg := "--preview='cat " + tempDir + "/{}.md'"
	fzfArgs := []string{
		headerArg,
		previewArg,
		"--preview-window=right:60%:wrap",
	}
	if cfg.Interactive.ModelPicker.Filter != "" {
		fzfArgs = append(fzfArgs, "--query="+shellQuote(cfg.Interactive.ModelPicker.Filter))
	}
	selected, cancelled, err := invokePicker(picker, candidates, fzfArgs...)
	if err != nil {
		return "", false, err
	}
	if cancelled {
		return "", true, nil
	}

	endpointID := strings.TrimSpace(selected)
	if endpointID == "" {
		return "", true, nil
	}
	return endpointID, false, nil
}

// writeModelDetails writes a prettified markdown file describing the model
// at <tempDir>/<endpoint_id>.md, creating intermediate directories as needed
// (endpoint_id contains slashes, e.g. fal-ai/flux/dev).
func writeModelDetails(tempDir string, m modelEntry) error {
	path := filepath.Join(tempDir, m.EndpointID+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	var sb strings.Builder
	if m.Metadata.DisplayName != "" {
		sb.WriteString(m.Metadata.DisplayName + "\n")
		sb.WriteString(strings.Repeat("-", len(m.Metadata.DisplayName)) + "\n\n")
	}
	sb.WriteString("ID:       " + m.EndpointID + "\n")
	if m.Metadata.Category != "" {
		sb.WriteString("Category: " + m.Metadata.Category + "\n")
	}
	if len(m.Metadata.Tags) > 0 {
		sb.WriteString("Tags:     " + strings.Join(m.Metadata.Tags, ", ") + "\n")
	}
	if m.Metadata.LicenseType != "" {
		sb.WriteString("Licence:  " + m.Metadata.LicenseType + "\n")
	}
	if m.Metadata.UpdatedAt != "" {
		sb.WriteString("Updated:  " + m.Metadata.UpdatedAt + "\n")
	}
	if m.Metadata.Description != "" {
		sb.WriteString("\n" + m.Metadata.Description + "\n")
	}
	// Plain URL -- iTerm2 / Terminal.app auto-linkify, no OSC 8 needed.
	if m.Metadata.ModelURL != "" {
		sb.WriteString("\nDocs: " + m.Metadata.ModelURL + "\n")
	} else {
		sb.WriteString("\nDocs: https://fal.ai/models/" + m.EndpointID + "\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// reorderPreselect moves the first entry whose EndpointID is matched by the
// preselect regex to the front of the slice. If preselect is empty, no entry
// matches, or the regex fails to compile, the slice is returned unchanged
// (with a stderr warning on compile failure). Stable for the remaining entries.
func reorderPreselect(models []modelEntry, preselect string) []modelEntry {
	if preselect == "" {
		return models
	}
	re, err := regexp.Compile(preselect)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: model-picker.preselect %q is not a valid regex: %v (proceeding without preselect)\n", preselect, err)
		return models
	}
	for i, m := range models {
		if re.MatchString(m.EndpointID) {
			if i == 0 {
				return models
			}
			out := make([]modelEntry, 0, len(models))
			out = append(out, m)
			out = append(out, models[:i]...)
			out = append(out, models[i+1:]...)
			return out
		}
	}
	return models
}

// fetchModels queries FAL's /v1/models endpoint for active models in the given category.
func fetchModels(falKey, category string) ([]modelEntry, error) {
	params := url.Values{}
	params.Set("category", category)
	params.Set("status", "active")
	params.Set("limit", "100")

	endpoint := fmt.Sprintf("%s/v1/models?%s", pricingBaseURL(), params.Encode())
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building /v1/models request: %w", err)
	}
	if falKey != "" {
		req.Header.Set("Authorization", "Key "+falKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching /v1/models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading /v1/models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FAL /v1/models returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed modelsListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing /v1/models response: %w", err)
	}
	return parsed.Models, nil
}
