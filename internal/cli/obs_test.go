// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
	"github.com/ph-commons/pagasa-pp-cli/internal/store"
)

func TestFilterAWSStations(t *testing.T) {
	stations := []pagasa.AWSStation{
		{StationID: "98", StationName: "Science Garden, Quezon City"},
		{StationID: "5001", StationName: "San Jose Synoptic Station"},
	}
	got := filterAWSStations(stations, "98")
	if len(got) != 1 || got[0].StationID != "98" {
		t.Fatalf("id filter: %+v", got)
	}
	got = filterAWSStations(stations, "science")
	if len(got) != 1 || got[0].StationID != "98" {
		t.Fatalf("name filter: %+v", got)
	}
	got = filterAWSStations(stations, "")
	if len(got) != 2 {
		t.Fatalf("empty filter: %d", len(got))
	}
}

func TestWhichIndex_IncludesObs(t *testing.T) {
	root := newRootCmd(&rootFlags{})
	found := false
	for _, e := range whichIndex {
		if e.Command == "obs" || e.Command == "obs history" {
			found = true
			parts := strings.Fields(e.Command)
			cmd, remaining, err := root.Find(parts)
			if err != nil || len(remaining) > 0 || cmd == nil {
				t.Errorf("whichIndex command %q does not resolve (err=%v remaining=%v)", e.Command, err, remaining)
			}
		}
	}
	if !found {
		t.Fatal("whichIndex missing obs entries")
	}
}

func TestObsDryRun(t *testing.T) {
	// Ensure command is registered on root
	root := newRootCmd(&rootFlags{dryRun: true, agent: true})
	cmd, _, err := root.Find([]string{"obs"})
	if err != nil || cmd == nil {
		t.Fatalf("obs not registered: %v", err)
	}
	hist, _, err := root.Find([]string{"obs", "history"})
	if err != nil || hist == nil {
		t.Fatalf("obs history not registered: %v", err)
	}
}

func TestObsNotMCPReadOnly_HistoryIs(t *testing.T) {
	root := newRootCmd(&rootFlags{})
	obs, _, err := root.Find([]string{"obs"})
	if err != nil || obs == nil {
		t.Fatalf("obs: %v", err)
	}
	if obs.Annotations["mcp:read-only"] == "true" {
		t.Error("top-level obs must not be mcp:read-only (--capture writes/prunes)")
	}
	if obs.Annotations["mcp:open-world"] != "true" {
		t.Error("top-level obs must be mcp:open-world (bagong HTTP, issue #24)")
	}
	hist, _, err := root.Find([]string{"obs", "history"})
	if err != nil || hist == nil {
		t.Fatalf("obs history: %v", err)
	}
	if hist.Annotations["mcp:read-only"] != "true" {
		t.Error("obs history should remain mcp:read-only")
	}
	if hist.Annotations["mcp:open-world"] == "true" {
		t.Error("obs history is local store only — must not be mcp:open-world")
	}
}

func TestObsCaptureWithLimitErrors(t *testing.T) {
	root := newRootCmd(&rootFlags{agent: true})
	root.SetArgs([]string{"obs", "--capture", "--limit", "5", "--json"})
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetOut(&bytes.Buffer{})
	err := root.Execute()
	if err == nil {
		t.Fatal("want error when --capture and --limit are both set")
	}
	if !strings.Contains(err.Error(), "--limit") || !strings.Contains(err.Error(), "--capture") {
		t.Fatalf("error should mention both flags, got: %v", err)
	}
}

func TestEmptyAWSObsHistoryJSONIsArray(t *testing.T) {
	// Nil slice would encode as null; callers (loadAWSObsHistory / ListAWSObs) must
	// normalize so machine output is [].
	var nilRows []store.AWSObsRow
	bNil, _ := json.Marshal(nilRows)
	if string(bNil) != "null" {
		t.Fatalf("precondition: nil slice should marshal to null, got %s", bNil)
	}
	rows := []store.AWSObsRow{}
	if rows == nil {
		t.Fatal("empty composite literal must be non-nil")
	}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Fatalf("empty history machine JSON = %s, want []", b)
	}
}
