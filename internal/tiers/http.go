package tiers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"
)

func readJSON(w http.ResponseWriter, r *http.Request, value any) bool {
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

func reply(w http.ResponseWriter, value any, err error, status int) {
	if err != nil {
		var pgerr *pgconn.PgError
		switch {
		case errors.Is(err, ErrInvalid):
			http.Error(w, "invalid request", http.StatusBadRequest)
		case errors.Is(err, sql.ErrNoRows):
			http.Error(w, "not found", http.StatusNotFound)
		case errors.Is(err, ErrConflict):
			http.Error(w, "version conflict", http.StatusConflict)
		case errors.As(err, &pgerr) && (pgerr.Code == "23505" || pgerr.Code == "23503"):
			http.Error(w, "conflict", http.StatusConflict)
		default:
			http.Error(w, "tier service unavailable", http.StatusInternalServerError)
		}
		return
	}
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// Register adds tier routes to a mux protected by the API authentication layer.
func Register(mux *http.ServeMux, db *sql.DB) {
	store := Store{DB: db}
	for _, item := range []struct{ path, axis string }{{"companies", "company"}, {"roles", "role"}} {
		item := item
		mux.HandleFunc("GET /api/v1/tiers/"+item.path, func(w http.ResponseWriter, r *http.Request) {
			values, err := store.ListTiers(r.Context(), item.axis)
			reply(w, values, err, http.StatusOK)
		})
		mux.HandleFunc("PUT /api/v1/tiers/"+item.path, func(w http.ResponseWriter, r *http.Request) {
			var value Tier
			if !readJSON(w, r, &value) {
				return
			}
			if value.Axis != "" && value.Axis != item.axis {
				reply(w, nil, ErrInvalid, 0)
				return
			}
			value.Axis = item.axis
			result, err := store.SaveTier(r.Context(), value)
			reply(w, result, err, http.StatusOK)
		})
	}
	mux.HandleFunc("GET /api/v1/tiers/{axis}/{tier}/history", func(w http.ResponseWriter, r *http.Request) {
		values, err := store.TierVersions(r.Context(), r.PathValue("axis"), r.PathValue("tier"))
		reply(w, values, err, http.StatusOK)
	})
	mux.HandleFunc("PUT /api/v1/role-families/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var value RoleFamily
		if !readJSON(w, r, &value) {
			return
		}
		if value.ID != 0 || value.TierID != "" {
			reply(w, nil, ErrInvalid, 0)
			return
		}
		value.ID = id
		reply(w, nil, store.UpdateRoleFamily(r.Context(), value), http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/role-families", func(w http.ResponseWriter, r *http.Request) {
		var value RoleFamily
		if !readJSON(w, r, &value) {
			return
		}
		if value.ID != 0 || value.TierID != "" {
			reply(w, nil, ErrInvalid, 0)
			return
		}
		result, err := store.CreateRoleFamily(r.Context(), value)
		reply(w, result, err, http.StatusCreated)
	})
	mux.HandleFunc("GET /api/v1/role-families", func(w http.ResponseWriter, r *http.Request) {
		after := int64(0)
		if raw := r.URL.Query().Get("after"); raw != "" {
			var err error
			after, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || after < 0 {
				reply(w, nil, ErrInvalid, 0)
				return
			}
		}
		result, err := store.ListRoleFamilies(r.Context(), after)
		reply(w, result, err, http.StatusOK)
	})
	for _, item := range []struct{ path, axis string }{{"companies", "company"}, {"role-families", "role"}} {
		item := item
		mux.HandleFunc("POST /api/v1/"+item.path+"/{id}/tier", func(w http.ResponseWriter, r *http.Request) {
			id, ok := pathID(w, r)
			if !ok {
				return
			}
			var value struct {
				TierID string `json:"tier_id"`
				Reason string `json:"reason"`
			}
			if !readJSON(w, r, &value) {
				return
			}
			reply(w, nil, store.Assign(r.Context(), item.axis, id, value.TierID, value.Reason), http.StatusNoContent)
		})
	}
	mux.HandleFunc("GET /api/v1/tier-assignments", func(w http.ResponseWriter, r *http.Request) {
		target, err := strconv.ParseInt(r.URL.Query().Get("target_id"), 10, 64)
		if err != nil {
			reply(w, nil, ErrInvalid, 0)
			return
		}
		after := int64(0)
		if raw := r.URL.Query().Get("after"); raw != "" {
			after, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				reply(w, nil, ErrInvalid, 0)
				return
			}
		}
		values, err := store.Assignments(r.Context(), r.URL.Query().Get("axis"), target, after)
		reply(w, values, err, http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/policies/global", func(w http.ResponseWriter, r *http.Request) {
		version, enabled, err := store.CurrentGlobal(r.Context())
		reply(w, map[string]any{"version": version, "enabled": enabled}, err, http.StatusOK)
	})
	mux.HandleFunc("PUT /api/v1/policies/global", func(w http.ResponseWriter, r *http.Request) {
		var value struct {
			Enabled *bool `json:"enabled"`
		}
		if !readJSON(w, r, &value) {
			return
		}
		if value.Enabled == nil {
			reply(w, nil, ErrInvalid, 0)
			return
		}
		version, err := store.SetGlobal(r.Context(), *value.Enabled)
		reply(w, map[string]int64{"version": version}, err, http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/policies/steps", func(w http.ResponseWriter, r *http.Request) {
		values, err := store.CurrentModes(r.Context())
		reply(w, values, err, http.StatusOK)
	})
	mux.HandleFunc("PUT /api/v1/policies/steps", func(w http.ResponseWriter, r *http.Request) {
		var change ModeChange
		if !readJSON(w, r, &change) {
			return
		}
		version, err := store.SetMode(r.Context(), change)
		reply(w, map[string]int64{"version": version}, err, http.StatusOK)
	})
}
