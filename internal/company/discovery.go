package company

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"karun.tech/jobagent/internal/platform/httpclient"
	"karun.tech/jobagent/internal/policy"
)

var boardKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,200}$`)
var embeddedURL = regexp.MustCompile(`https?://[^\s"'<>\\]+`)

// Candidate identifies a board discovered from a careers page.
type Candidate struct {
	Provider string
	Key      string
}

// Inspector finds board references without running a browser or model.
type Inspector struct {
	DB    *sql.DB
	fetch func(context.Context, string, []string, func(context.Context, string) error) (int, []byte, string, error)
}

// NewInspector creates a policy-gated careers-page inspector.
func NewInspector(db *sql.DB) *Inspector {
	return &Inspector{DB: db, fetch: func(ctx context.Context, target string, hosts []string, authorize func(context.Context, string) error) (int, []byte, string, error) {
		client, err := httpclient.New(hosts, authorize)
		if err != nil {
			return 0, nil, "", err
		}
		defer client.Close()
		return client.GetPage(ctx, target)
	}}
}

func candidates(page []byte, pageURL string) ([]Candidate, bool, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, false, errors.New("invalid page URL")
	}
	root, err := html.Parse(strings.NewReader(string(page)))
	if err != nil {
		return nil, false, fmt.Errorf("parse careers page: %w", err)
	}
	var found []Candidate
	seen := make(map[Candidate]bool)
	hasScript := false
	add := func(raw string) {
		ref, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return
		}
		u := base.ResolveReference(ref)
		if u.Scheme != "https" || u.User != nil {
			return
		}
		host := strings.ToLower(u.Hostname())
		var provider string
		switch host {
		case "boards.greenhouse.io", "job-boards.greenhouse.io":
			provider = "greenhouse"
		case "jobs.ashbyhq.com":
			provider = "ashby"
		default:
			return
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		key := parts[0]
		if provider == "greenhouse" && u.Path == "/embed/job_board" {
			key = u.Query().Get("for")
		}
		if !boardKeyPattern.MatchString(key) {
			return
		}
		candidate := Candidate{Provider: provider, Key: key}
		if !seen[candidate] {
			seen[candidate] = true
			found = append(found, candidate)
		}
	}
	stack := []*html.Node{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node.Type == html.ElementNode {
			if node.Data == "script" {
				hasScript = true
			}
			for _, attr := range node.Attr {
				if attr.Key == "href" || attr.Key == "src" {
					add(attr.Val)
				}
				for _, raw := range embeddedURL.FindAllString(attr.Val, -1) {
					add(raw)
				}
			}
		}
		if node.Type == html.TextNode {
			for _, raw := range embeddedURL.FindAllString(node.Data, -1) {
				add(raw)
			}
		}
		for child := node.LastChild; child != nil; child = child.PrevSibling {
			stack = append(stack, child)
		}
	}
	return found, hasScript, nil
}

func insertException(ctx context.Context, tx *sql.Tx, seed Seed, reason string) error {
	result, err := tx.ExecContext(ctx, `INSERT INTO browser_exceptions (seed_id,company_id,reason)
		VALUES ($1,$2,$3) ON CONFLICT (seed_id,reason) WHERE status='PENDING' DO NOTHING`,
		seed.ID, seed.CompanyID, reason)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		return auditMutation(ctx, tx, "browser_exception_queued", seed.CompanyID)
	}
	return nil
}

func (i *Inspector) queueException(ctx context.Context, seed Seed, reason string) error {
	tx, err := i.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertException(ctx, tx, seed, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// Discover inspects one registered seed and persists candidates or exceptions.
func (i *Inspector) Discover(ctx context.Context, seedID int64) ([]Candidate, error) {
	store := Store{DB: i.DB}
	seed, err := store.GetSeed(ctx, seedID)
	if err != nil {
		return nil, err
	}
	if err := policy.CheckGlobal(ctx, i.DB); err != nil {
		if errors.Is(err, policy.ErrGlobalStopped) {
			if queueErr := i.queueException(ctx, seed, "global_policy_blocked"); queueErr != nil {
				return nil, queueErr
			}
		}
		return nil, err
	}
	target, err := url.Parse(seed.URL)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil {
		return nil, errors.New("seed must be an HTTPS URL")
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	if err := policy.CheckSource(ctx, i.DB, host, policy.Discovery); err != nil {
		if errors.Is(err, policy.ErrCapabilityDenied) || errors.Is(err, policy.ErrSourceUnregistered) {
			if queueErr := i.queueException(ctx, seed, "source_policy_blocked"); queueErr != nil {
				return nil, queueErr
			}
		}
		return nil, err
	}
	rows, err := i.DB.QueryContext(ctx, "SELECT source FROM source_policies WHERE discovery_allowed")
	if err != nil {
		return nil, err
	}
	var hosts []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			rows.Close()
			return nil, err
		}
		hosts = append(hosts, source)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	status, body, finalURL, err := i.fetch(ctx, seed.URL, hosts, func(ctx context.Context, host string) error {
		return policy.CheckSource(ctx, i.DB, host, policy.Discovery)
	})
	if err != nil {
		if errors.Is(err, httpclient.ErrRobotsDenied) || errors.Is(err, policy.ErrCapabilityDenied) || errors.Is(err, policy.ErrSourceUnregistered) {
			if queueErr := i.queueException(ctx, seed, "source_policy_blocked"); queueErr != nil {
				return nil, queueErr
			}
		}
		return nil, err
	}
	if status == http.StatusForbidden || status == http.StatusUnauthorized {
		if err := i.queueException(ctx, seed, "site_blocked"); err != nil {
			return nil, err
		}
		return nil, errors.New("careers page is blocked")
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("careers page returned status %d", status)
	}
	found, hasScript, err := candidates(body, finalURL)
	if err != nil {
		return nil, err
	}
	final, err := url.Parse(finalURL)
	if err != nil {
		return nil, err
	}
	final.RawQuery, final.Fragment, final.User = "", "", nil
	provenance, err := json.Marshal(map[string]any{"seed_id": seed.ID, "source_url": final.String()})
	if err != nil {
		return nil, err
	}
	tx, err := i.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, candidate := range found {
		var id int64
		err := tx.QueryRowContext(ctx, `INSERT INTO ats_boards (company_id,provider,region,board_key,provenance)
			VALUES ($1,$2,'',$3,$4) ON CONFLICT (provider,region,board_key) DO NOTHING RETURNING id`,
			seed.CompanyID, candidate.Provider, candidate.Key, provenance).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			var owner int64
			if err := tx.QueryRowContext(ctx, "SELECT company_id FROM ats_boards WHERE provider=$1 AND region='' AND board_key=$2", candidate.Provider, candidate.Key).Scan(&owner); err != nil {
				return nil, err
			}
			if owner != seed.CompanyID {
				if err := insertException(ctx, tx, seed, "board_identity_conflict"); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := auditMutation(ctx, tx, "board_discovered", seed.CompanyID); err != nil {
			return nil, err
		}
	}
	if len(found) == 0 && hasScript {
		if err := insertException(ctx, tx, seed, "dynamic_review"); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return found, nil
}
