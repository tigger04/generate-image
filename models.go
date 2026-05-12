// ABOUTME: Model picker flow for --pick-model.
// ABOUTME: Fetches FAL /v1/models, presents to the configured picker, returns the selected endpoint_id.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type modelEntry struct {
	EndpointID string `json:"endpoint_id"`
	Metadata   struct {
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Status      string `json:"status"`
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

	candidates := make([]string, 0, len(models))
	for _, m := range models {
		line := m.EndpointID
		if m.Metadata.DisplayName != "" {
			line += "\t" + m.Metadata.DisplayName
		}
		if m.Metadata.Description != "" {
			line += "\t" + m.Metadata.Description
		}
		candidates = append(candidates, line)
	}

	selected, cancelled, err := invokePicker(picker, candidates)
	if err != nil {
		return "", false, err
	}
	if cancelled {
		return "", true, nil
	}

	endpointID := strings.SplitN(selected, "\t", 2)[0]
	endpointID = strings.TrimSpace(endpointID)
	if endpointID == "" {
		return "", true, nil
	}
	return endpointID, false, nil
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
