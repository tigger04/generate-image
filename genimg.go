// ABOUTME: generate subcommand handler (alias: gen) -- generates or edits images via the FAL API.
// ABOUTME: Reads prompt from stdin, writes image to specified output path. Supports reference images.

package main

import (
	"bufio"
	"encoding/base64"
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
)

const maxRefImages = 3

func printGenImgUsage(subcommandName string) {
	fmt.Fprintln(os.Stderr, "Usage: pix generate [flags] [reference-images...] <output-file>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Alias: gen")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Reads a text prompt from stdin and generates an image via the FAL API.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "If earlier positional arguments are existing image files, they are sent")
	fmt.Fprintln(os.Stderr, "as references to the model's edit endpoint (max 3). The last positional")
	fmt.Fprintln(os.Stderr, "is always the target output filename.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  -h, --help          Show this help message")
	fmt.Fprintln(os.Stderr, "  --dry-run           Show what would happen without calling the API")
	fmt.Fprintln(os.Stderr, "  -p, --preview       Open the image after generation")
	fmt.Fprintln(os.Stderr, "  --load-prompt       Pick a saved prompt via fzf (or configured picker)")
	fmt.Fprintln(os.Stderr, "  --no-load-prompt    Disable load-prompt mode for this invocation")
	fmt.Fprintln(os.Stderr, "  --pick-model        Pick a FAL model from the catalogue via the picker")
	fmt.Fprintln(os.Stderr, "  --no-pick-model     Disable model-picker mode for this invocation")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Global flags (place before subcommand):")
	fmt.Fprintln(os.Stderr, "  -q, --quiet         Suppress non-error output")
}

// runGenImg handles the generate subcommand (alias: gen). globalQuiet is the
// value of the global --quiet flag parsed before the subcommand.
// subcommandName is the name the user typed; it is only used to make error
// messages reference the invocation the user actually wrote.
func runGenImg(args []string, globalQuiet bool, subcommandName string) int {
	dryRun := false
	preview := false
	helpRequested := false
	loadPromptFlag := false
	noLoadPromptFlag := false
	pickModelFlag := false
	noPickModelFlag := false
	var positionals []string

	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			helpRequested = true
		case "--dry-run":
			dryRun = true
		case "-p", "--preview":
			preview = true
		case "--load-prompt":
			loadPromptFlag = true
		case "--no-load-prompt":
			noLoadPromptFlag = true
		case "--pick-model":
			pickModelFlag = true
		case "--no-pick-model":
			noPickModelFlag = true
		case "-q", "--quiet":
			fmt.Fprintln(os.Stderr, "Error: --quiet is a global flag and must be placed before the subcommand")
			fmt.Fprintf(os.Stderr, "       (try: pix --quiet %s ...)\n", subcommandName)
			return 2
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
				printGenImgUsage(subcommandName)
				return 2
			}
			positionals = append(positionals, arg)
		}
	}

	// --help is mutually exclusive with all other args/flags.
	if helpRequested {
		hasOther := dryRun || preview || loadPromptFlag || noLoadPromptFlag || pickModelFlag || noPickModelFlag || len(positionals) > 0
		if hasOther {
			fmt.Fprintln(os.Stderr, "Error: --help cannot be combined with other flags or arguments")
			printGenImgUsage(subcommandName)
			return 2
		}
		printGenImgUsage(subcommandName)
		return 0
	}

	if len(positionals) == 0 {
		printGenImgUsage(subcommandName)
		return 2
	}

	if globalQuiet && dryRun {
		fmt.Fprintln(os.Stderr, "Error: --quiet and --dry-run cannot be used together")
		return 2
	}

	// Last positional is target; earlier ones are reference images.
	outputPath := positionals[len(positionals)-1]
	refs := positionals[:len(positionals)-1]

	if len(refs) > maxRefImages {
		fmt.Fprintf(os.Stderr, "Error: maximum %d reference images supported (got %d)\n", maxRefImages, len(refs))
		return 1
	}

	for _, ref := range refs {
		if err := validateRefImage(ref); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	}

	confDir, err := resolveConfDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	cfg, err := loadConfig(filepath.Join(confDir, "config.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	useLoadPrompt := !noLoadPromptFlag && (loadPromptFlag || cfg.LoadPrompt.Always)

	var prompt string
	if useLoadPrompt && isStdinTTY() {
		result, err := runLoadPromptFlow(cfg, globalQuiet)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if result.Cancelled {
			return 0
		}
		prompt = result.Prompt
	} else {
		// Either load-prompt is inactive, or it is active but stdin is piped.
		// Piped stdin with content overrides the load-prompt flow -- the user's
		// piped content is taken as the prompt directly.
		prompt, err = readPrompt()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			return 1
		}
		if prompt == "" {
			fmt.Fprintln(os.Stderr, "Error: no prompt provided on stdin")
			return 1
		}
	}

	falKey, err := resolveFALKey(cfg, confDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	baseURL := falBaseURL()

	// Resolve which model endpoint to call. By default, use cfg.Model with
	// the existing /edit suffix when refs are present. If --pick-model (or
	// model-picker.always) is active, prompt the user to select from FAL's
	// /v1/models catalogue and use the selected endpoint_id as-is.
	usePickModel := !noPickModelFlag && (pickModelFlag || cfg.ModelPicker.Always)
	pickedEndpoint := ""
	if usePickModel {
		if !isStdinTTY() {
			fmt.Fprintln(os.Stderr, "Error: --pick-model requires an interactive terminal")
			return 2
		}
		ep, cancelled, err := runModelPickerFlow(cfg, falKey, len(refs) > 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if cancelled {
			return 0
		}
		pickedEndpoint = ep
	}

	// Build endpoint and payload depending on whether refs are present.
	endpoint := cfg.Model
	if pickedEndpoint != "" {
		endpoint = pickedEndpoint
	}
	payload := map[string]interface{}{"prompt": prompt}
	if len(refs) > 0 {
		if pickedEndpoint == "" {
			// Default behaviour for cfg.Model: append /edit suffix when refs present.
			endpoint = cfg.Model + "/edit"
		}
		uris := make([]string, 0, len(refs))
		for _, ref := range refs {
			uri, err := refToDataURI(ref)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			uris = append(uris, uri)
		}
		payload["image_urls"] = uris
	}

	if dryRun {
		url := fmt.Sprintf("%s/%s", baseURL, endpoint)

		// For dry-run output, replace base64 data with filename references for readability.
		displayPayload := map[string]interface{}{"prompt": prompt}
		if len(refs) > 0 {
			displayURLs := make([]string, 0, len(refs))
			for _, ref := range refs {
				displayURLs = append(displayURLs, fmt.Sprintf("<base64 of %s>", ref))
			}
			displayPayload["image_urls"] = displayURLs
		}
		pretty, err := json.MarshalIndent(displayPayload, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to marshal dry-run payload: %v\n", err)
			return 1
		}

		fmt.Fprintf(os.Stderr, "POST %s\n", url)
		fmt.Fprintf(os.Stderr, "%s\n", pretty)
		fmt.Fprintf(os.Stderr, "Output: %s\n", outputPath)
		fmt.Fprintln(os.Stderr, "(dry run -- no API call made)")
		return 0
	}

	previewCmd := cfg.PreviewCommand
	if preview && previewCmd == "" {
		previewCmd = defaultPreviewCommand()
	}

	// Print reference image warnings before the API call (unless quiet).
	if !globalQuiet {
		for _, ref := range refs {
			fmt.Fprintf(os.Stderr, "⚠️  Using %s as reference image (will be sent to FAL)\n", ref)
		}
	}

	client := &http.Client{Timeout: 120 * time.Second}

	imageData, contentType, err := generateImageWithPayload(client, baseURL, endpoint, payload, falKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	result, err := writeImage(imageData, contentType, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	outputPath = result.Path

	if !globalQuiet {
		reportCost(client, cfg.Model, falKey)
		if result.Converted {
			fmt.Fprintf(os.Stderr, "Wrote %s (converted %s to %s)\n", result.Path, result.FromFmt, result.ToFmt)
		} else {
			fmt.Fprintf(os.Stderr, "Wrote %s\n", result.Path)
		}
	}

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

// readPrompt reads a prompt from stdin. If stdin is a TTY (interactive),
// it prompts the user and reads a single line. If stdin is piped, it reads all input.
func readPrompt() (string, error) {
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		// TTY: prompt the user, then read one line.
		fmt.Fprintln(os.Stderr, "Interactive terminal detected. Type your prompt and press Enter:")
		fmt.Fprint(os.Stderr, "> ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}

	// Piped: read all.
	bytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes)), nil
}

// validateRefImage confirms the path exists, is a regular file, and has a
// recognised image extension.
func validateRefImage(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("reference image %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("reference image %s is a directory", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return nil
	default:
		return fmt.Errorf("reference image %s: unrecognised extension %s (supported: .jpg, .jpeg, .png, .webp, .gif)", path, ext)
	}
}

// refToDataURI reads an image file and returns it as a base64-encoded data URI.
func refToDataURI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading reference %s: %w", path, err)
	}
	mime := mimeFromExt(filepath.Ext(path))
	encoded := base64.StdEncoding.EncodeToString(data)
	return "data:" + mime + ";base64," + encoded, nil
}

// mimeFromExt maps a file extension to a MIME type.
func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
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

// writeResult holds the outcome of writeImage for status reporting.
type writeResult struct {
	Path      string
	Converted bool
	FromFmt   string
	ToFmt     string
}

// writeImage writes image data to disk, handling extension logic:
//   - No extension: appends the API format extension
//   - Matching extension: writes as-is
//   - Mismatched extension: converts via ImageMagick or returns an error
//
// Returns the final output path (which may differ from the input).
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

	tmpFile, err := os.CreateTemp("", "pix-*"+srcExt)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(imageData); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	cmd := exec.Command(magickPath, tmpPath, outputPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("magick conversion failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	return nil
}
