// Command job-agent provides entry points for scheduled discovery tasks.
package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) != 2 || args[1] != "--once" {
		fmt.Fprintln(os.Stderr, "usage: job-agent {discover-companies|poll-boards|discovery-monitor} --once")
		return 2
	}
	switch args[0] {
	case "discover-companies", "poll-boards", "discovery-monitor":
		fmt.Fprintf(os.Stderr, "%s: implementation is not yet available\n", args[0])
		return 1
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		return 2
	}
}
