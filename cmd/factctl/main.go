package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	// Define root flags
	var (
		showVersion = flag.Bool("version", false, "Show version information")
		headless    = flag.Bool("headless", false, "Run Factorio in headless mode")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: factctl <command> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  up      Create or update an instance\n")
		fmt.Fprintf(os.Stderr, "  down    Remove an instance\n")
		fmt.Fprintf(os.Stderr, "  run     Launch Factorio with the specified instance\n")
		fmt.Fprintf(os.Stderr, "  logs    Stream instance logs\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("factctl version %s\n", version)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]
	switch command {
	case "up":
		// TODO: Implement instance creation/update
		fmt.Println("Creating/updating instance...")
	case "down":
		// TODO: Implement instance removal
		fmt.Println("Removing instance...")
	case "run":
		// TODO: Implement Factorio launch
		fmt.Printf("Launching Factorio (headless=%v)...\n", *headless)
	case "logs":
		// TODO: Implement log streaming
		fmt.Println("Streaming logs...")
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}