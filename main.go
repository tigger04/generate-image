// ABOUTME: CLI tool that generates images from text prompts via the FAL API.
// ABOUTME: Reads prompt from stdin, writes image to the path given as $1.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const version = "0.1.0"

type apiKeyConfig struct {
	Command string `yaml:"command"`
	File    string `yaml:"file"`
}

type config struct {
	Model          string                  `yaml:"model"`
	APIKeys        map[string]apiKeyConfig `yaml:"api-keys"`
	PreviewCommand string                  `yaml:"preview-command"`
}

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

func main() {
	os.Exit(run())
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: generate-image [flags] <output-file>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Reads a text prompt from stdin and generates an image via the FAL API.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  -h, --help       Show this help message")
	fmt.Fprintln(os.Stderr, "  --version        Show version")
	fmt.Fprintln(os.Stderr, "  -q, --quiet      Suppress cost output")
	fmt.Fprintln(os.Stderr, "  --dry-run        Show what would happen without calling the API")
	fmt.Fprintln(os.Stderr, "  -p, --preview    Open the image after generation (requires preview_command in config)")
}

func run() int {
	// Parse flags and positional arguments.
	quiet := false
	dryRun := false
	preview := false
	var outputPath string

	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "--help":
			printUsage()
			return 0
		case "--version":
			fmt.Fprintln(os.Stderr, "generate-image "+version)
			return 0
		case "-q", "--quiet":
			quiet = true
		case "--dry-run":
			dryRun = true
		case "-p", "--preview":
			preview = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
				printUsage()
				return 2
			}
			if outputPath == "" {
				outputPath = arg
			} else {
				fmt.Fprintln(os.Stderr, "Error: too many arguments")
				printUsage()
				return 2
			}
		}
	}

	if outputPath == "" {
		printUsage()
		return 2
	}

	if quiet && dryRun {
		fmt.Fprintln(os.Stderr, "Error: --quiet and --dry-run cannot be used together")
		return 2
	}

	// Read prompt from stdin.
	stdinBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		return 1
	}
	prompt := strings.TrimSpace(string(stdinBytes))
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "Error: no prompt provided on stdin")
		return 1
	}

	// Resolve binary directory for config files.
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving executable path: %v\n", err)
		return 1
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving symlinks: %v\n", err)
		return 1
	}
	binDir := filepath.Dir(exePath)
	confDir := configDir(binDir)

	// Load config first (needed for api-keys resolution).
	cfg, err := loadConfig(filepath.Join(confDir, "config.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Resolve FAL API key via priority chain:
	// 1. FAL_KEY env var
	// 2. api-keys.fal.command in config
	// 3. api-keys.fal.file in config
	// 4. .env fallback in config dir
	falKey, err := resolveFALKey(cfg, confDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Dry-run: show what would be sent and exit.
	if dryRun {
		baseURL := os.Getenv("FAL_BASE_URL")
		if baseURL == "" {
			baseURL = "https://fal.run"
		}
		url := fmt.Sprintf("%s/%s", baseURL, cfg.Model)

		payload := map[string]string{"prompt": prompt}
		pretty, _ := json.MarshalIndent(payload, "", "  ")

		fmt.Fprintf(os.Stderr, "POST %s\n", url)
		fmt.Fprintf(os.Stderr, "%s\n", pretty)
		fmt.Fprintf(os.Stderr, "Output: %s\n", outputPath)
		fmt.Fprintln(os.Stderr, "(dry run -- no API call made)")
		return 0
	}

	// Resolve preview command before making the API call.
	previewCmd := cfg.PreviewCommand
	if preview && previewCmd == "" {
		previewCmd = defaultPreviewCommand()
	}

	// Determine API base URL (test hook via env var).
	baseURL := os.Getenv("FAL_BASE_URL")
	if baseURL == "" {
		baseURL = "https://fal.run"
	}

	// Call FAL API.
	client := &http.Client{Timeout: 120 * time.Second}

	imageData, contentType, err := generateImage(client, baseURL, cfg.Model, prompt, falKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Write image to disk, handling extension and format conversion.
	result, err := writeImage(imageData, contentType, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	outputPath = result.Path

	// Report status (unless --quiet).
	if !quiet {
		reportCost(client, baseURL, cfg.Model, falKey)
		if result.Converted {
			fmt.Fprintf(os.Stderr, "Wrote %s (converted %s to %s)\n", result.Path, result.FromFmt, result.ToFmt)
		} else {
			fmt.Fprintf(os.Stderr, "Wrote %s\n", result.Path)
		}
	}

	// Preview the image if requested.
	if preview {
		cmd := exec.Command("sh", "-c", previewCmd+" \"$1\"", "--", outputPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running preview command: %v\n", err)
			return 1
		}
	}

	return 0
}

// defaultPreviewCommand returns the platform-appropriate image viewer command.
func defaultPreviewCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "open"
	case "windows":
		return "cmd /c start \"\""
	default:
		return "xdg-open"
	}
}

// resolveFALKey resolves the FAL API key via priority chain:
// 1. FAL_KEY environment variable
// 2. api-keys.fal.command (run command, use stdout)
// 3. api-keys.fal.file (read key from file)
// 4. .env fallback in config directory
func resolveFALKey(cfg *config, confDir string) (string, error) {
	// 1. Environment variable
	if key := os.Getenv("FAL_KEY"); key != "" {
		return key, nil
	}

	// 2 & 3. Config-driven sources
	if falCfg, ok := cfg.APIKeys["fal"]; ok {
		// 2. Command (takes priority over file)
		if falCfg.Command != "" {
			// Trust boundary: command is from user-owned config.yaml, not external input.
			out, err := exec.Command("sh", "-c", falCfg.Command).Output()
			if err != nil {
				return "", fmt.Errorf("api-keys.fal.command failed: %w", err)
			}
			key := strings.TrimSpace(string(out))
			if key != "" {
				return key, nil
			}
		}

		// 3. File
		if falCfg.File != "" {
			data, err := os.ReadFile(falCfg.File)
			if err != nil {
				return "", fmt.Errorf("api-keys.fal.file: %w", err)
			}
			key := strings.TrimSpace(string(data))
			if key != "" {
				return key, nil
			}
		}
	}

	// 4. .env fallback
	return loadFALKey(filepath.Join(confDir, ".env"))
}

// configDir returns the directory containing .env and config.yaml.
// It checks the binary directory first (development), then falls back
// to ~/.config/generate-image/ (installed via make install).
func configDir(binDir string) string {
	if hasConfigFiles(binDir) {
		return binDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return binDir
	}
	candidate := filepath.Join(home, ".config", "generate-image")
	if hasConfigFiles(candidate) {
		return candidate
	}
	// Fall back to binDir so error messages reference the expected location.
	return binDir
}

// hasConfigFiles returns true if the directory contains config.yaml or .env.
func hasConfigFiles(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
		return true
	}
	return false
}

// loadFALKey reads FAL_KEY from a .env file.
func loadFALKey(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read .env: %w (expected FAL_KEY in %s)", err, path)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(key) == "FAL_KEY" {
			v := strings.TrimSpace(value)
			if v != "" {
				return v, nil
			}
		}
	}

	return "", fmt.Errorf("FAL_KEY not found in %s", path)
}

// writeImage writes image data to disk, handling extension logic:
// - No extension: appends the API format extension
// - Matching extension: writes as-is
// - Mismatched extension: converts via ImageMagick or returns an error
// Returns the final output path (which may differ from the input).
// writeResult holds the outcome of writeImage for status reporting.
type writeResult struct {
	Path      string
	Converted bool
	FromFmt   string
	ToFmt     string
}

func writeImage(imageData []byte, contentType string, outputPath string) (*writeResult, error) {
	apiExt := extFromContentType(contentType)
	userExt := filepath.Ext(outputPath)

	if userExt == "" {
		outputPath = outputPath + apiExt
	}

	if userExt == "" || strings.EqualFold(userExt, apiExt) {
		if err := os.WriteFile(outputPath, imageData, 0644); err != nil {
			return nil, fmt.Errorf("writing output file: %w", err)
		}
		return &writeResult{Path: outputPath}, nil
	}

	// Mismatched extension -- try to convert with magick.
	if err := convertWithMagick(imageData, apiExt, outputPath); err != nil {
		return nil, err
	}
	return &writeResult{
		Path:      outputPath,
		Converted: true,
		FromFmt:   strings.TrimPrefix(apiExt, "."),
		ToFmt:     strings.TrimPrefix(userExt, "."),
	}, nil
}

// extFromContentType maps a Content-Type to a file extension (with dot).
func extFromContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	// Strip parameters (e.g. "image/jpeg; charset=...")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".jpg"
	}
}

// convertWithMagick converts image data to the format implied by outputPath
// using ImageMagick's magick command. Returns an error if magick is not
// available or conversion fails.
func convertWithMagick(imageData []byte, srcExt string, outputPath string) error {
	magickPath, err := exec.LookPath("magick")
	if err != nil {
		apiFormat := strings.TrimPrefix(srcExt, ".")
		userFormat := strings.TrimPrefix(filepath.Ext(outputPath), ".")
		return fmt.Errorf(
			"API returned %s but you requested %s; install ImageMagick (magick) to convert automatically",
			apiFormat, userFormat,
		)
	}

	// Write source image to temp file.
	tmpFile, err := os.CreateTemp("", "generate-image-*"+srcExt)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(imageData); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Convert with magick.
	cmd := exec.Command(magickPath, tmpPath, outputPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("magick conversion failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// loadConfig reads config.yaml.
func loadConfig(path string) (*config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config.yaml: %w (expected config at %s)", err, path)
	}

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config.yaml: %w", err)
	}

	if cfg.Model == "" {
		return nil, fmt.Errorf("config.yaml: 'model' field is required")
	}

	return &cfg, nil
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

	// Download the image.
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

// reportCost fetches pricing from the FAL API and prints cost to stderr.
// Pricing is best-effort; failures are non-fatal and silently ignored.
func reportCost(client *http.Client, baseURL, model, falKey string) {
	// Determine pricing URL -- use FAL_BASE_URL if set (for tests),
	// otherwise use the real pricing endpoint.
	pricingBase := baseURL
	if pricingBase == "https://fal.run" {
		pricingBase = "https://api.fal.ai"
	}

	url := fmt.Sprintf("%s/v1/models/pricing?endpoint_id=%s", pricingBase, model)
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
