// ABOUTME: pix CLI entry point.
// ABOUTME: Parses global flags, dispatches to subcommand handlers (gen-img, cost).

package main

import (
	"fmt"
	"os"
	"strings"
)

const version = "0.3.0"

func main() {
	os.Exit(run())
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: pix [global flags] <subcommand> [subcommand args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "A minimal CLI for generating images via the FAL API.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  gen-img   Generate an image from a text prompt (stdin)")
	fmt.Fprintln(os.Stderr, "  cost      Query pricing for the configured model")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Global flags (placed before the subcommand):")
	fmt.Fprintln(os.Stderr, "  -h, --help       Show this help message")
	fmt.Fprintln(os.Stderr, "  --version        Show version")
	fmt.Fprintln(os.Stderr, "  -q, --quiet      Suppress non-error output")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'pix <subcommand> --help' for subcommand-specific usage.")
}

func run() int {
	args := os.Args[1:]

	// No args: print usage and exit zero (help-first convention).
	if len(args) == 0 {
		printUsage()
		return 0
	}

	// Parse global flags up to the first non-flag (subcommand) or end of args.
	quiet := false
	helpRequested := false
	versionRequested := false
	subcommandIdx := -1

	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			subcommandIdx = i
			break
		}
		switch arg {
		case "-h", "--help":
			helpRequested = true
		case "--version":
			versionRequested = true
		case "-q", "--quiet":
			quiet = true
		default:
			// Non-global flag in the global position.
			fmt.Fprintf(os.Stderr, "Error: %s is not a global flag (must be placed after the subcommand)\n", arg)
			printUsage()
			return 2
		}
	}

	// --help is mutually exclusive with all other flags and arguments.
	if helpRequested {
		hasOther := versionRequested || quiet || subcommandIdx >= 0
		if hasOther {
			fmt.Fprintln(os.Stderr, "Error: --help cannot be combined with other flags or arguments")
			printUsage()
			return 2
		}
		printUsage()
		return 0
	}

	// --version takes precedence over subcommand dispatch (but not over --help).
	if versionRequested {
		if quiet || subcommandIdx >= 0 {
			fmt.Fprintln(os.Stderr, "Error: --version cannot be combined with other flags or arguments")
			printUsage()
			return 2
		}
		fmt.Println("pix " + version)
		return 0
	}

	// No subcommand provided after global flags.
	if subcommandIdx < 0 {
		printUsage()
		return 2
	}

	subcommand := args[subcommandIdx]
	subcommandArgs := args[subcommandIdx+1:]

	switch subcommand {
	case "gen-img":
		return runGenImg(subcommandArgs, quiet)
	case "cost":
		return runCost(subcommandArgs, quiet)
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcommand)
		printUsage()
		return 2
	}
}
