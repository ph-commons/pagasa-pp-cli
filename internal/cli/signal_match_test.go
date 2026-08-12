// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
)

func TestMatchWindSignal(t *testing.T) {
	signals := []pagasa.WindSignal{
		{Signal: 1, AffectedAreas: "Luzon: Pangasinan, La Union"},
		{Signal: 2, AffectedAreas: "Luzon: Cagayan, Isabela, Metro Manila"},
		{Signal: 3, AffectedAreas: "Luzon: Batanes, Babuyan Islands"},
	}
	cases := []struct {
		area   string
		wantN  int
		wantOK bool
	}{
		{"Mandaluyong", 0, false},
		{"Metro Manila", 2, true},
		{"cagayan", 2, true},
		{"Batanes", 3, true},
		{"", 0, false},
	}
	for _, tc := range cases {
		n, _, ok := matchWindSignal(signals, tc.area)
		if ok != tc.wantOK || n != tc.wantN {
			t.Fatalf("matchWindSignal(%q) = (%d, ok=%v); want (%d, ok=%v)",
				tc.area, n, ok, tc.wantN, tc.wantOK)
		}
	}
	if n, _, ok := matchWindSignal(nil, "Manila"); ok || n != 0 {
		t.Fatalf("empty signals should not match")
	}
}
