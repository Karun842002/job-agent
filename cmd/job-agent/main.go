// Command job-agent provides entry points for scheduled discovery tasks.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type fixture struct {
	Companies     []string     `json:"companies"`
	AsOf          time.Time    `json:"as_of"`
	MaxAgeSeconds int64        `json:"max_age_seconds"`
	Runs          []fixtureRun `json:"runs"`
}

type fixtureRun struct {
	Job        string    `json:"job"`
	Outcome    string    `json:"outcome"`
	FinishedAt time.Time `json:"finished_at"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: job-agent {discover-companies|poll-boards|discovery-monitor} --once [--fixture path]")
		return 2
	}
	command := args[0]
	if command != "discover-companies" && command != "poll-boards" && command != "discovery-monitor" {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		return 2
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	once := flags.Bool("once", false, "run once")
	fixturePath := flags.String("fixture", "", "run a local synthetic fixture")
	if err := flags.Parse(args[1:]); err != nil || !*once || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "a single --once invocation is required")
		return 2
	}
	if *fixturePath == "" {
		fmt.Fprintf(os.Stderr, "%s: production implementation is not yet available\n", command)
		return 1
	}
	file, err := os.Open(*fixturePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open fixture: %v\n", err)
		return 1
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var data fixture
	if err := decoder.Decode(&data); err != nil {
		fmt.Fprintf(os.Stderr, "invalid fixture: %v\n", err)
		return 1
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		fmt.Fprintln(os.Stderr, "fixture must contain one JSON object")
		return 1
	}
	switch command {
	case "discover-companies":
		fmt.Printf("synthetic discovery: %d companies\n", len(data.Companies))
	case "poll-boards":
		fmt.Println("synthetic polling: no external requests")
	case "discovery-monitor":
		if data.AsOf.IsZero() || data.MaxAgeSeconds <= 0 || data.MaxAgeSeconds > 86400*365 {
			fmt.Fprintln(os.Stderr, "invalid monitor fixture")
			return 1
		}
		for _, job := range []string{"discover-companies", "poll-boards"} {
			healthy := false
			for _, run := range data.Runs {
				age := data.AsOf.Sub(run.FinishedAt)
				if run.Job == job && run.Outcome == "success" && age >= 0 && age <= time.Duration(data.MaxAgeSeconds)*time.Second {
					healthy = true
				}
			}
			if !healthy {
				fmt.Fprintf(os.Stderr, "synthetic monitor: missing or stale success for %s\n", job)
				return 1
			}
		}
		fmt.Println("synthetic monitor: healthy")
	}
	return 0
}
