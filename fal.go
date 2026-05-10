// ABOUTME: FAL API HTTP client helpers.
// ABOUTME: Image generation, unit pricing, and historical cost estimate calls.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type falResponse struct {
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

type pricingResponse struct {
	Prices []struct {
		UnitPrice float64 `json:"unit_price"`
		Unit      string  `json:"unit"`
		Currency  string  `json:"currency"`
	} `json:"prices"`
}

type estimateResponse struct {
	TotalCost float64 `json:"total_cost"`
	Currency  string  `json:"currency"`
}

// falBaseURL returns the FAL generation base URL, honouring the FAL_BASE_URL
// env var for tests.
func falBaseURL() string {
	if v := os.Getenv("FAL_BASE_URL"); v != "" {
		return v
	}
	return "https://fal.run"
}

// pricingBaseURL returns the FAL pricing API base URL. If FAL_BASE_URL is set
// (test mode), it is used for both generation and pricing. Otherwise the
// production pricing host is used.
func pricingBaseURL() string {
	base := falBaseURL()
	if base == "https://fal.run" {
		return "https://api.fal.ai"
	}
	return base
}

// generateImage calls the FAL API and returns the image bytes and content type.
func generateImage(client *http.Client, baseURL, model, prompt, falKey string) ([]byte, string, error) {
	reqBody, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		return nil, "", fmt.Errorf("failed to build request: %w", err)
	}

	url := fmt.Sprintf("%s/%s", baseURL, model)
	req, err := http.NewRequest("POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+falKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("FAL API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read FAL API response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("FAL API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var falResp falResponse
	if err := json.Unmarshal(body, &falResp); err != nil {
		return nil, "", fmt.Errorf("failed to parse FAL API response: %w", err)
	}

	if len(falResp.Images) == 0 {
		return nil, "", fmt.Errorf("FAL API returned no images")
	}

	imgResp, err := client.Get(falResp.Images[0].URL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download image: %w", err)
	}
	defer imgResp.Body.Close()

	imageData, err := io.ReadAll(imgResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image data: %w", err)
	}

	contentType := imgResp.Header.Get("Content-Type")
	return imageData, contentType, nil
}

// fetchUnitPrice queries the FAL unit pricing endpoint.
func fetchUnitPrice(client *http.Client, pricingBase, model, falKey string) (float64, string, error) {
	url := fmt.Sprintf("%s/v1/models/pricing?endpoint_id=%s", pricingBase, model)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Key "+falKey)

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}

	var pricing pricingResponse
	if err := json.Unmarshal(body, &pricing); err != nil {
		return 0, "", err
	}

	if len(pricing.Prices) == 0 {
		return 0, "", fmt.Errorf("no pricing data")
	}

	return pricing.Prices[0].UnitPrice, pricing.Prices[0].Unit, nil
}

// fetchHistoricalEstimate queries the FAL historical cost estimation endpoint.
func fetchHistoricalEstimate(client *http.Client, pricingBase, model, falKey string) (float64, error) {
	url := fmt.Sprintf("%s/v1/models/pricing/estimate", pricingBase)

	reqBody, err := json.Marshal(map[string]interface{}{
		"estimate_type": "historical_api_price",
		"endpoints": map[string]interface{}{
			model: map[string]interface{}{
				"call_quantity": 1,
			},
		},
	})
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+falKey)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var estimate estimateResponse
	if err := json.Unmarshal(body, &estimate); err != nil {
		return 0, err
	}

	return estimate.TotalCost, nil
}

// reportCost fetches pricing from the FAL API and prints cost to stderr.
// Pricing is best-effort; failures are non-fatal and silently ignored.
func reportCost(client *http.Client, model, falKey string) {
	url := fmt.Sprintf("%s/v1/models/pricing?endpoint_id=%s", pricingBaseURL(), model)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Key "+falKey)

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var pricing pricingResponse
	if err := json.Unmarshal(body, &pricing); err != nil {
		return
	}

	if len(pricing.Prices) == 0 {
		return
	}

	price := pricing.Prices[0]
	fmt.Fprintf(os.Stderr, "Cost: $%.2f (unit: %s) for model %s (source: FAL API)\n", price.UnitPrice, price.Unit, model)
}
