// Command admin runs safe, deterministic one-shot registry operations.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"karun.tech/jobagent/internal/company"
)

type app struct {
	db       *sql.DB
	out      io.Writer
	discover func(context.Context, int64) ([]company.Candidate, error)
	validate func(context.Context, int64) (company.ValidationResult, error)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mainRun(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func mainRun(ctx context.Context, args []string, out io.Writer) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	return (app{db: db, out: out}).run(ctx, args)
}

func (a app) run(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: admin companies import | boards discover|validate|list")
	}
	switch {
	case args[0] == "companies" && args[1] == "import":
		flags := flag.NewFlagSet("companies import", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		format := flags.String("format", "", "input format: csv or yaml")
		filePath := flags.String("file", "", "path to import file")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || (*format != "csv" && *format != "yaml") || *filePath == "" {
			return errors.New("usage: admin companies import --format csv|yaml --file path")
		}
		file, err := os.Open(*filePath)
		if err != nil {
			return fmt.Errorf("open import file: %w", err)
		}
		defer file.Close()
		count, err := (company.Store{DB: a.db}).Import(ctx, *format, file)
		if err != nil {
			return err
		}
		return a.writeJSON(map[string]int{"imported": count})
	case args[0] == "boards" && args[1] == "discover":
		flags := flag.NewFlagSet("boards discover", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		seedID := flags.String("seed", "", "registered seed ID")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		id, err := positiveID(*seedID)
		if err != nil || flags.NArg() != 0 {
			return errors.New("usage: admin boards discover --seed id")
		}
		discover := a.discover
		if discover == nil {
			discover = company.NewInspector(a.db).Discover
		}
		values, err := discover(ctx, id)
		if err != nil {
			return err
		}
		return a.writeJSON(map[string]any{"candidates": values})
	case args[0] == "boards" && args[1] == "validate":
		flags := flag.NewFlagSet("boards validate", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		boardID := flags.String("id", "", "registered board ID")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		id, err := positiveID(*boardID)
		if err != nil || flags.NArg() != 0 {
			return errors.New("usage: admin boards validate --id id")
		}
		validate := a.validate
		if validate == nil {
			validate = company.NewValidator(a.db).Validate
		}
		value, err := validate(ctx, id)
		if err != nil {
			return err
		}
		return a.writeJSON(value)
	case args[0] == "boards" && args[1] == "list":
		flags := flag.NewFlagSet("boards list", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		after := flags.Int64("after", 0, "return boards after this ID")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *after < 0 {
			return errors.New("usage: admin boards list [--after id]")
		}
		values, err := (company.Store{DB: a.db}).ListBoards(ctx, *after)
		if err != nil {
			return err
		}
		return a.writeJSON(values)
	default:
		return errors.New("unknown operation; usage: admin companies import | boards discover|validate|list")
	}
}

func positiveID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("ID must be a positive integer")
	}
	return id, nil
}

func (a app) writeJSON(value any) error {
	encoder := json.NewEncoder(a.out)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write command result: %w", err)
	}
	return nil
}
