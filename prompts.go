// ABOUTME: Saved-prompt selection flow for --load-prompt.
// ABOUTME: Enumerates files under load-prompt.path, invokes the configured picker, and assembles the final prompt.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type loadPromptResult struct {
	Prompt    string
	Cancelled bool
}

func runLoadPromptFlow(cfg *config, globalQuiet bool) (*loadPromptResult, error) {
	lp := cfg.LoadPrompt

	if lp.Path == "" {
		return nil, fmt.Errorf("load-prompt.path is not configured in config.yaml")
	}

	picker := effectivePicker(cfg)

	fields := strings.Fields(picker)
	if len(fields) == 0 {
		return nil, fmt.Errorf("load-prompt.picker is empty")
	}
	pickerBin := fields[0]
	expandedBin, err := expandTilde(pickerBin)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath(expandedBin); err != nil {
		return nil, fmt.Errorf("load-prompt picker %q not found on PATH: %w", pickerBin, err)
	}

	resolvedPath, err := expandTilde(lp.Path)
	if err != nil {
		return nil, err
	}

	files, err := listPromptFiles(resolvedPath)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("load-prompt directory %s contains no prompt files (empty)", lp.Path)
	}

	selected, cancelled, err := invokePicker(picker, files)
	if err != nil {
		return nil, err
	}
	if cancelled {
		return &loadPromptResult{Cancelled: true}, nil
	}

	baseBytes, err := os.ReadFile(selected)
	if err != nil {
		return nil, fmt.Errorf("reading selected prompt %s: %w", selected, err)
	}
	base := strings.TrimRight(string(baseBytes), " \t\r\n")

	if !globalQuiet {
		fmt.Fprintln(os.Stderr, "Selected prompt:")
		fmt.Fprintln(os.Stderr, base)
		fmt.Fprint(os.Stderr, "Add to prompt (Enter to send as-is): ")
	}

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading additional text from stdin: %w", err)
	}
	addition := strings.TrimSpace(line)

	final := base
	if addition != "" {
		final = base + "\n\n" + addition
	}

	return &loadPromptResult{Prompt: final}, nil
}

func listPromptFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("load-prompt.path %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("load-prompt.path %s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading load-prompt directory %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

func invokePicker(pickerCmd string, candidates []string) (string, bool, error) {
	cmd := exec.Command("sh", "-c", pickerCmd)
	cmd.Stderr = os.Stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", false, fmt.Errorf("creating picker stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", false, fmt.Errorf("starting picker: %w", err)
	}

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		defer stdin.Close()
		for _, c := range candidates {
			if _, err := fmt.Fprintln(stdin, c); err != nil {
				return
			}
		}
	}()

	err = cmd.Wait()
	<-writeDone

	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", true, nil
		}
		return "", false, fmt.Errorf("picker invocation failed: %w", err)
	}

	selected := strings.TrimSpace(stdout.String())
	if selected == "" {
		return "", true, nil
	}
	return selected, false, nil
}

// expandTilde resolves a leading '~/' prefix to the user's home directory.
// Bare '~' and other tilde forms (~user/...) are returned unchanged.
func expandTilde(p string) (string, error) {
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expanding ~/: %w", err)
	}
	return filepath.Join(home, p[2:]), nil
}

func isStdinTTY() bool {
	if os.Getenv("PIX_TEST_TTY") == "1" {
		return true
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
