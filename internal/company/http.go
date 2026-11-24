package company

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"karun.tech/jobagent/internal/policy"
)

// Handler exposes registry operations using a server-configured bearer token.
func Handler(store Store, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/companies", func(w http.ResponseWriter, r *http.Request) {
		after, ok := afterID(w, r)
		if !ok {
			return
		}
		values, err := store.ListCompanies(r.Context(), after)
		respond(w, values, err, http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/seeds", func(w http.ResponseWriter, r *http.Request) {
		after, ok := afterID(w, r)
		if !ok {
			return
		}
		values, err := store.ListSeeds(r.Context(), after)
		respond(w, values, err, http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/boards", func(w http.ResponseWriter, r *http.Request) {
		after, ok := afterID(w, r)
		if !ok {
			return
		}
		values, err := store.ListBoards(r.Context(), after)
		respond(w, values, err, http.StatusOK)
	})
	mux.HandleFunc("POST /api/v1/companies", func(w http.ResponseWriter, r *http.Request) {
		var value Company
		if !decode(w, r, &value) {
			return
		}
		value.ID = 0
		result, err := store.CreateCompany(r.Context(), value)
		respond(w, result, err, http.StatusCreated)
	})
	mux.HandleFunc("GET /api/v1/companies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		value, err := store.GetCompany(r.Context(), id)
		respond(w, value, err, http.StatusOK)
	})
	mux.HandleFunc("PUT /api/v1/companies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		var value Company
		if !decode(w, r, &value) {
			return
		}
		value.ID = id
		respond(w, nil, store.UpdateCompany(r.Context(), value), http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/companies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		respond(w, nil, store.DeleteCompany(r.Context(), id), http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/seeds", func(w http.ResponseWriter, r *http.Request) {
		var value Seed
		if !decode(w, r, &value) {
			return
		}
		value.ID = 0
		result, err := store.CreateSeed(r.Context(), value)
		respond(w, result, err, http.StatusCreated)
	})
	mux.HandleFunc("GET /api/v1/seeds/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		value, err := store.GetSeed(r.Context(), id)
		respond(w, value, err, http.StatusOK)
	})
	mux.HandleFunc("PUT /api/v1/seeds/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		var value Seed
		if !decode(w, r, &value) {
			return
		}
		value.ID = id
		respond(w, nil, store.UpdateSeed(r.Context(), value), http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/seeds/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		respond(w, nil, store.DeleteSeed(r.Context(), id), http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/boards", func(w http.ResponseWriter, r *http.Request) {
		var value Board
		if !decode(w, r, &value) {
			return
		}
		if !emptyMetadata(value.Metadata) {
			http.Error(w, "set metadata through its dedicated endpoint", http.StatusBadRequest)
			return
		}
		value.ID = 0
		result, err := store.CreateBoard(r.Context(), value)
		respond(w, result, err, http.StatusCreated)
	})
	mux.HandleFunc("GET /api/v1/boards/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		value, err := store.GetBoard(r.Context(), id)
		respond(w, value, err, http.StatusOK)
	})
	mux.HandleFunc("PUT /api/v1/boards/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		var value Board
		if !decode(w, r, &value) {
			return
		}
		if !emptyMetadata(value.Metadata) {
			http.Error(w, "set metadata through its dedicated endpoint", http.StatusBadRequest)
			return
		}
		value.ID = id
		respond(w, nil, store.UpdateBoard(r.Context(), value), http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/boards/{id}/validate", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		result, err := NewValidator(store.DB).Validate(r.Context(), id)
		respond(w, result, err, http.StatusOK)
	})
	mux.HandleFunc("PUT /api/v1/boards/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		var value BoardMetadata
		if !decode(w, r, &value) {
			return
		}
		respond(w, nil, store.SetBoardMetadata(r.Context(), id, value), http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/boards/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		respond(w, nil, store.DeleteBoard(r.Context(), id), http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/companies/import", func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		count, err := store.Import(r.Context(), format, http.MaxBytesReader(w, r.Body, maxImportBytes+1))
		respond(w, map[string]int{"imported": count}, err, http.StatusCreated)
	})
	expected := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.Error(w, "API authentication is not configured", http.StatusServiceUnavailable)
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		provided := ""
		validHeader := len(parts) == 2 && strings.EqualFold(parts[0], "Bearer")
		if validHeader {
			provided = parts[1]
		}
		actual := sha256.Sum256([]byte(provided))
		if !validHeader || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func emptyMetadata(value BoardMetadata) bool {
	return value.State == "" && len(value.Capabilities) == 0 && len(value.Provenance) == 0 &&
		value.LastVerifiedAt == nil && len(value.OperatorOverride) == 0
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func afterID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	value := r.URL.Query().Get("after")
	if value == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 0 {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func requestID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func respond(w http.ResponseWriter, value any, err error, success int) {
	if err != nil {
		var pgerr *pgconn.PgError
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.Error(w, "not found", http.StatusNotFound)
		case errors.As(err, &pgerr) && (pgerr.Code == "23505" || pgerr.Code == "23503"):
			http.Error(w, "registry conflict", http.StatusConflict)
		case errors.Is(err, policy.ErrCapabilityDenied), errors.Is(err, policy.ErrSourceUnregistered), errors.Is(err, policy.ErrGlobalStopped):
			http.Error(w, "source policy denies validation", http.StatusForbidden)
		case errors.Is(err, ErrInvalidInput):
			http.Error(w, "invalid request", http.StatusBadRequest)
		default:
			http.Error(w, "registry unavailable", http.StatusInternalServerError)
		}
		return
	}
	if success == http.StatusNoContent {
		w.WriteHeader(success)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(success)
	_ = json.NewEncoder(w).Encode(value)
}
