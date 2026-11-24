package company

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxImportBytes = 1 << 20

type importCompany struct {
	Name       string `yaml:"name"`
	Domain     string `yaml:"domain"`
	CareersURL string `yaml:"careers_url"`
}

// Import atomically imports company seeds from CSV or YAML.
func (s Store) Import(ctx context.Context, format string, input io.Reader) (int, error) {
	data, err := io.ReadAll(io.LimitReader(input, maxImportBytes+1))
	if err != nil {
		return 0, err
	}
	if len(data) > maxImportBytes {
		return 0, fmt.Errorf("%w: import exceeds size limit", ErrInvalidInput)
	}
	var records []importCompany
	switch format {
	case "csv":
		reader := csv.NewReader(bytes.NewReader(data))
		rows, err := reader.ReadAll()
		if err != nil {
			return 0, fmt.Errorf("%w: parse CSV", ErrInvalidInput)
		}
		if len(rows) == 0 || len(rows[0]) != 3 || rows[0][0] != "name" || rows[0][1] != "domain" || rows[0][2] != "careers_url" {
			return 0, fmt.Errorf("%w: CSV header must be name,domain,careers_url", ErrInvalidInput)
		}
		for _, row := range rows[1:] {
			records = append(records, importCompany{Name: row[0], Domain: row[1], CareersURL: row[2]})
		}
	case "yaml":
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&records); err != nil {
			return 0, fmt.Errorf("%w: parse YAML", ErrInvalidInput)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return 0, fmt.Errorf("%w: YAML must contain one document", ErrInvalidInput)
		}
	default:
		return 0, fmt.Errorf("%w: unsupported import format", ErrInvalidInput)
	}
	if len(records) == 0 {
		return 0, fmt.Errorf("%w: import has no companies", ErrInvalidInput)
	}
	for i := range records {
		records[i].Name = strings.TrimSpace(records[i].Name)
		records[i].Domain = strings.ToLower(strings.TrimSpace(records[i].Domain))
		records[i].CareersURL = strings.TrimSpace(records[i].CareersURL)
		if records[i].Name == "" || records[i].Domain == "" || records[i].CareersURL == "" {
			return 0, fmt.Errorf("%w: row %d requires name, domain and careers URL", ErrInvalidInput, i+1)
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for i, record := range records {
		var id int64
		if err := tx.QueryRowContext(ctx, "INSERT INTO companies (name,domain) VALUES ($1,$2) RETURNING id", record.Name, record.Domain).Scan(&id); err != nil {
			return 0, fmt.Errorf("row %d: insert company: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO company_seeds (company_id,url) VALUES ($1,$2)", id, record.CareersURL); err != nil {
			return 0, fmt.Errorf("row %d: insert seed: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tier_assignments(axis,target_id,tier_id,tier_version,actor,reason)
			SELECT 'company',$1,id,version,'system','initial_default' FROM policy_tiers WHERE axis='company' AND id='UNASSIGNED'`, id); err != nil {
			return 0, err
		}
		if err := auditMutation(ctx, tx, "company_imported", id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(records), nil
}
