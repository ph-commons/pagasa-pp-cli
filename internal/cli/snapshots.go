// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"time"

	"github.com/ph-commons/pagasa-pp-cli/internal/store"
)

// snapshotType is the resources.resource_type key for persisted weather snapshots.
const snapshotType = "pagasa_snapshot"

// snapshot is one point-in-time capture of the PAGASA synopsis + city forecasts.
// history and drift read these back; now/storm/digest write them best-effort.
type snapshot struct {
	CapturedAt string          `json:"captured_at"` // RFC3339
	Synopsis   string          `json:"synopsis,omitempty"`
	StormName  string          `json:"storm_name,omitempty"`
	StormKind  string          `json:"storm_kind,omitempty"`
	Cities     json.RawMessage `json:"cities,omitempty"`
}

// saveSnapshot persists a snapshot keyed by its capture time. Best-effort: any
// error is returned to the caller, which typically logs and continues — a
// failed cache write must never break a live read command.
func saveSnapshot(ctx context.Context, dbPath string, s snapshot) error {
	if s.CapturedAt == "" {
		s.CapturedAt = time.Now().UTC().Format(time.RFC3339)
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Upsert(snapshotType, s.CapturedAt, data)
}

// loadSnapshots returns persisted snapshots newest-first. Returns an empty
// slice (not an error) when no local mirror exists yet.
func loadSnapshots(ctx context.Context, dbPath string) ([]snapshot, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = ?`, snapshotType)
	if err != nil {
		return nil, err
	}
	var out []snapshot
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var s snapshot
		if json.Unmarshal(raw, &s) == nil {
			out = append(out, s)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CapturedAt > out[j].CapturedAt })
	return out, nil
}
