// ABOUTME: gen-img subcommand handler -- generates images from text prompts.
// ABOUTME: Reads prompt from stdin, writes image to specified output path.

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
)

func printGenImgUsage() {
	fmt.Fprintln(os.Stderr, "Usage: pix gen-img [flags] <output-file>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Reads a text prompt from stdin and generates an image via the FAL API.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  -h, --help       Show this help message")
	fmt.Fprintln(os.Stderr, "  --dry-run        Show what would happen without calling the API")
	fmt.Fprintln(os.Stderr, "  -p, --preview    Open the image after generation")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Global flags (place before subcommand):")
	fmt.Fprintln(os.Stderr, "  -q, --quiet      Suppress non-error output")
}

// runGenImg handles the gen-img subcommand. globalQuiet is the value of the
// global --quiet flag parsed before the subcommand.
func runGenImg(args []string, globalQuiet bool) int {
	dryRun := false
	preview := false
	helpRequested := false
	var outputPath string

	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			helpRequested = true
		case "--dry-run":
			dryRun = true
		case "-p", "--preview":
			preview = true
		case "-q", "--quiet":
			fmt.Fprintln(os.Stderr, "Error: --quiet is a global flag and must be placed before the subcommand")
			fmt.Fprintln(os.Stderr, "       (try: pix --quiet gen-img ...)")
			return 2
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
				printGenImgUsage()
				return 2
			}
			if outputPath == "" {
				outputPath = arg
			} else {
				fmt.Fprintln(os.Stderr, "Error: too many arguments")
				printGenImgUsage()
				return 2
			}
		}
	}

	// --help is mutually exclusive with all other args/flags.
	if helpRequested {
		hasOther := dryRun || preview || outputPath != ""
		if hasOther {
			fmt.Fprintln(os.Stderr, "Error: --help cannot be combined with other flags or arguments")
			printGenImgUsage()
			return 2
		}
		printGenImgUsage()
		return 0
	}

	if outputPath == "" {
		printGenImgUsage()
		return 2
	}

	if globalQuiet && dryRun {
		fmt.Fprintln(os.Stderr, "Error: --quiet and --dry-run cannot be used together")
		return 2
	}

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

	falKey, err := resolveFALKey(cfg, confDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	baseURL := falBaseURL()

	if dryRun {
		url := fmt.Sprintf("%s/%s", baseURL, cfg.Model)

		payload := map[string]string{"prompt": prompt}
		pretty, err := json.MarshalIndent(payload, "", "  ")
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

	client := &http.Client{Timeout: 120 * time.Second}

	imageData, contentType, err := generateImage(client, baseURL, cfg.Model, prompt, falKey)
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
