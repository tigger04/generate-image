// ABOUTME: cost subcommand handler -- queries FAL pricing without generating an image.
// ABOUTME: Calls both the unit price and historical estimate endpoints.

package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func printCostUsage() {
	fmt.Fprintln(os.Stderr, "Usage: pix cost [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Queries pricing for the configured model without generating an image.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  -h, --help       Show this help message")
	fmt.Fprintln(os.Stderr, "  --dry-run        Show what URLs would be queried without making API calls")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Global flags (place before subcommand):")
	fmt.Fprintln(os.Stderr, "  -q, --quiet      Suppress output (exits zero with no stdout/stderr)")
}

// runCost handles the cost subcommand. globalQuiet is the value of the global
// --quiet flag parsed before the subcommand.
func runCost(args []string, globalQuiet bool) int {
	dryRun := false
	helpRequested := false

	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			helpRequested = true
		case "--dry-run":
			dryRun = true
		case "-q", "--quiet":
			fmt.Fprintln(os.Stderr, "Error: --quiet is a global flag and must be placed before the subcommand")
			fmt.Fprintln(os.Stderr, "       (try: pix --quiet cost ...)")
			return 2
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
				printCostUsage()
				return 2
			}
			fmt.Fprintf(os.Stderr, "Error: unexpected argument: %s\n", arg)
			printCostUsage()
			return 2
		}
	}

	// --help is mutually exclusive with all other flags.
	if helpRequested {
		if dryRun {
			fmt.Fprintln(os.Stderr, "Error: --help cannot be combined with other flags")
			printCostUsage()
			return 2
		}
		printCostUsage()
		return 0
	}

	if globalQuiet {
		// Quiet mode: skip everything (no API call, no output).
		return 0
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

	pricingBase := pricingBaseURL()

	if dryRun {
		fmt.Fprintf(os.Stderr, "Model: %s\n", cfg.Model)
		fmt.Fprintf(os.Stderr, "Would GET %s/v1/models/pricing?endpoint_id=%s\n", pricingBase, cfg.Model)
		fmt.Fprintf(os.Stderr, "Would POST %s/v1/models/pricing/estimate (historical_api_price for %s)\n", pricingBase, cfg.Model)
		fmt.Fprintln(os.Stderr, "(dry run -- no API calls made)")
		return 0
	}

	falKey, err := resolveFALKey(cfg, confDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	client := &http.Client{Timeout: 30 * time.Second}

	fmt.Fprintf(os.Stderr, "Model: %s\n", cfg.Model)

	unitPrice, unit, err := fetchUnitPrice(client, pricingBase, cfg.Model, falKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unit price: not available (%v)\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Unit price: $%.2f per %s (source: FAL API)\n", unitPrice, unit)
	}

	estimate, err := fetchHistoricalEstimate(client, pricingBase, cfg.Model, falKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Estimated cost: not available (no usage history for this model)")
	} else {
		fmt.Fprintf(os.Stderr, "Estimated cost: $%.4f per call based on usage history (source: FAL API)\n", estimate)
	}

	return 0
}
