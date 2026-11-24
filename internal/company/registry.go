// Package company stores company seeds and ATS board mappings.
package company

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
	"karun.tech/jobagent/internal/audit"
)

// Company is a canonical employer.
type Company struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
	TierID string `json:"tier_id"`
}

// Seed is a company-owned careers page or other curated discovery seed.
type Seed struct {
	ID        int64  `json:"id"`
	CompanyID int64  `json:"company_id"`
	URL       string `json:"url"`
}

// Board identifies an ATS board belonging to a company.
type Board struct {
	ID        int64         `json:"id"`
	CompanyID int64         `json:"company_id"`
	Provider  string        `json:"provider"`
	Region    string        `json:"region"`
	Key       string        `json:"board_key"`
	Metadata  BoardMetadata `json:"metadata"`
}

// BoardMetadata keeps observed data and operator overrides separate.
type BoardMetadata struct {
	State            string          `json:"state"`
	Capabilities     json.RawMessage `json:"capabilities"`
	Provenance       json.RawMessage `json:"provenance"`
	LastVerifiedAt   *time.Time      `json:"last_verified_at"`
	OperatorOverride json.RawMessage `json:"operator_override"`
}

// ErrInvalidInput identifies invalid registry input.
var ErrInvalidInput = errors.New("invalid registry input")

// Store is a PostgreSQL-backed registry.
type Store struct{ DB *sql.DB }

func auditMutation(ctx context.Context, tx *sql.Tx, name string, companyID int64) error {
	event := audit.Event{Name: name, Actor: "operator", Cause: "registry_change", CompanyID: &companyID}
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		traceID := span.TraceID().String()
		event.TraceID = &traceID
	}
	_, err := audit.Append(ctx, tx, event)
	return err
}

// CreateCompany adds an employer and an audit event atomically.
func (s Store) CreateCompany(ctx context.Context, company Company) (Company, error) {
	company.Name, company.Domain = strings.TrimSpace(company.Name), strings.ToLower(strings.TrimSpace(company.Domain))
	if company.Name == "" || company.Domain == "" {
		return Company{}, fmt.Errorf("%w: name and domain are required", ErrInvalidInput)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Company{}, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, "INSERT INTO companies (name, domain) VALUES ($1,$2) RETURNING id", company.Name, company.Domain).Scan(&company.ID)
	if err != nil {
		return Company{}, fmt.Errorf("create company: %w", err)
	}
	company.TierID = "UNASSIGNED"
	if _, err := tx.ExecContext(ctx, `INSERT INTO tier_assignments(axis,target_id,tier_id,tier_version,actor,reason)
		SELECT 'company',$1,id,version,'system','initial_default' FROM policy_tiers WHERE axis='company' AND id='UNASSIGNED'`, company.ID); err != nil {
		return Company{}, err
	}
	if err := auditMutation(ctx, tx, "company_created", company.ID); err != nil {
		return Company{}, err
	}
	if err := tx.Commit(); err != nil {
		return Company{}, err
	}
	return company, nil
}

// GetCompany reads a company by ID.
func (s Store) GetCompany(ctx context.Context, id int64) (Company, error) {
	var company Company
	err := s.DB.QueryRowContext(ctx, "SELECT id, name, domain, company_tier_id FROM companies WHERE id=$1", id).Scan(&company.ID, &company.Name, &company.Domain, &company.TierID)
	return company, err
}

// UpdateCompany changes an employer and records the change atomically.
func (s Store) UpdateCompany(ctx context.Context, company Company) error {
	company.Name, company.Domain = strings.TrimSpace(company.Name), strings.ToLower(strings.TrimSpace(company.Domain))
	if company.ID <= 0 || company.Name == "" || company.Domain == "" {
		return fmt.Errorf("%w: id, name and domain are required", ErrInvalidInput)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE companies SET name=$2, domain=$3 WHERE id=$1", company.ID, company.Name, company.Domain)
	if err != nil {
		return fmt.Errorf("update company: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if err := auditMutation(ctx, tx, "company_updated", company.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteCompany deletes a company only when no seeds or boards refer to it.
func (s Store) DeleteCompany(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "DELETE FROM companies WHERE id=$1", id)
	if err != nil {
		return fmt.Errorf("delete company: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if err := auditMutation(ctx, tx, "company_deleted", id); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateSeed adds a discovery seed to a company.
func (s Store) CreateSeed(ctx context.Context, seed Seed) (Seed, error) {
	seed.URL = strings.TrimSpace(seed.URL)
	if seed.CompanyID <= 0 || seed.URL == "" {
		return Seed{}, fmt.Errorf("%w: company and URL are required", ErrInvalidInput)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Seed{}, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "INSERT INTO company_seeds (company_id,url) VALUES ($1,$2) RETURNING id", seed.CompanyID, seed.URL).Scan(&seed.ID); err != nil {
		return Seed{}, fmt.Errorf("create seed: %w", err)
	}
	if err := auditMutation(ctx, tx, "seed_created", seed.CompanyID); err != nil {
		return Seed{}, err
	}
	if err := tx.Commit(); err != nil {
		return Seed{}, err
	}
	return seed, nil
}

// GetSeed returns a discovery seed.
func (s Store) GetSeed(ctx context.Context, id int64) (Seed, error) {
	var seed Seed
	err := s.DB.QueryRowContext(ctx, "SELECT id,company_id,url FROM company_seeds WHERE id=$1", id).Scan(&seed.ID, &seed.CompanyID, &seed.URL)
	return seed, err
}

// UpdateSeed updates a discovery seed.
func (s Store) UpdateSeed(ctx context.Context, seed Seed) error {
	seed.URL = strings.TrimSpace(seed.URL)
	if seed.ID <= 0 || seed.CompanyID <= 0 || seed.URL == "" {
		return fmt.Errorf("%w: id, company and URL are required", ErrInvalidInput)
	}
	return s.change(ctx, "seed_updated", "UPDATE company_seeds SET company_id=$2,url=$3 WHERE id=$1 RETURNING company_id", seed.ID, seed.CompanyID, seed.URL)
}

// DeleteSeed removes a discovery seed.
func (s Store) DeleteSeed(ctx context.Context, id int64) error {
	return s.change(ctx, "seed_deleted", "DELETE FROM company_seeds WHERE id=$1 RETURNING company_id", id)
}

// CreateBoard adds a provider board without overwriting other boards.
func (s Store) CreateBoard(ctx context.Context, board Board) (Board, error) {
	board.Provider, board.Region, board.Key = strings.TrimSpace(board.Provider), strings.TrimSpace(board.Region), strings.TrimSpace(board.Key)
	if board.CompanyID <= 0 || board.Provider == "" || board.Key == "" {
		return Board{}, fmt.Errorf("%w: company, provider and board key are required", ErrInvalidInput)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Board{}, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, "INSERT INTO ats_boards (company_id,provider,region,board_key) VALUES ($1,$2,$3,$4) RETURNING id", board.CompanyID, board.Provider, board.Region, board.Key).Scan(&board.ID)
	if err != nil {
		return Board{}, fmt.Errorf("create board: %w", err)
	}
	if err := auditMutation(ctx, tx, "board_created", board.CompanyID); err != nil {
		return Board{}, err
	}
	if err := tx.Commit(); err != nil {
		return Board{}, err
	}
	board.Metadata = BoardMetadata{State: "candidate", Capabilities: json.RawMessage("{}"), Provenance: json.RawMessage("{}"), OperatorOverride: json.RawMessage("{}")}
	return board, nil
}

// GetBoard reads a board by ID.
func (s Store) GetBoard(ctx context.Context, id int64) (Board, error) {
	var board Board
	err := s.DB.QueryRowContext(ctx, "SELECT id,company_id,provider,region,board_key,state,capabilities,provenance,last_verified_at,operator_override FROM ats_boards WHERE id=$1", id).Scan(&board.ID, &board.CompanyID, &board.Provider, &board.Region, &board.Key, &board.Metadata.State, &board.Metadata.Capabilities, &board.Metadata.Provenance, &board.Metadata.LastVerifiedAt, &board.Metadata.OperatorOverride)
	return board, err
}

// UpdateBoard updates the board identity and ownership.
func (s Store) UpdateBoard(ctx context.Context, board Board) error {
	board.Provider, board.Region, board.Key = strings.TrimSpace(board.Provider), strings.TrimSpace(board.Region), strings.TrimSpace(board.Key)
	if board.ID <= 0 || board.CompanyID <= 0 || board.Provider == "" || board.Key == "" {
		return fmt.Errorf("%w: id, company, provider and board key are required", ErrInvalidInput)
	}
	return s.change(ctx, "board_updated", "UPDATE ats_boards SET company_id=$2,provider=$3,region=$4,board_key=$5 WHERE id=$1 RETURNING company_id", board.ID, board.CompanyID, board.Provider, board.Region, board.Key)
}

// DeleteBoard removes a board.
func (s Store) DeleteBoard(ctx context.Context, id int64) error {
	return s.change(ctx, "board_deleted", "DELETE FROM ats_boards WHERE id=$1 RETURNING company_id", id)
}

func (s Store) change(ctx context.Context, event, query string, args ...any) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var companyID int64
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&companyID); err != nil {
		return fmt.Errorf("change registry: %w", err)
	}
	if err := auditMutation(ctx, tx, event, companyID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListCompanies returns up to 100 companies after the supplied ID.
func (s Store) ListCompanies(ctx context.Context, after int64) ([]Company, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,name,domain,company_tier_id FROM companies WHERE id>$1 ORDER BY id LIMIT 100", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	companies := make([]Company, 0)
	for rows.Next() {
		var value Company
		if err := rows.Scan(&value.ID, &value.Name, &value.Domain, &value.TierID); err != nil {
			return nil, err
		}
		companies = append(companies, value)
	}
	return companies, rows.Err()
}

// ListSeeds returns up to 100 seeds after the supplied ID.
func (s Store) ListSeeds(ctx context.Context, after int64) ([]Seed, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,company_id,url FROM company_seeds WHERE id>$1 ORDER BY id LIMIT 100", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seeds := make([]Seed, 0)
	for rows.Next() {
		var value Seed
		if err := rows.Scan(&value.ID, &value.CompanyID, &value.URL); err != nil {
			return nil, err
		}
		seeds = append(seeds, value)
	}
	return seeds, rows.Err()
}

// ListBoards returns up to 100 boards after the supplied ID.
func (s Store) ListBoards(ctx context.Context, after int64) ([]Board, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,company_id,provider,region,board_key,state,capabilities,provenance,last_verified_at,operator_override FROM ats_boards WHERE id>$1 ORDER BY id LIMIT 100", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	boards := make([]Board, 0)
	for rows.Next() {
		var value Board
		if err := rows.Scan(&value.ID, &value.CompanyID, &value.Provider, &value.Region, &value.Key, &value.Metadata.State, &value.Metadata.Capabilities, &value.Metadata.Provenance, &value.Metadata.LastVerifiedAt, &value.Metadata.OperatorOverride); err != nil {
			return nil, err
		}
		boards = append(boards, value)
	}
	return boards, rows.Err()
}
