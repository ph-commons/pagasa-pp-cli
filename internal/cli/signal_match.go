// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
)

// matchWindSignal finds the highest signal number whose affected_areas text
// contains area (case-insensitive substring). Returns 0 when no row matches.
// Empty wind_signals is "not confirmed," not "confirmed clear."
func matchWindSignal(signals []pagasa.WindSignal, area string) (signal int, areas string, ok bool) {
	needle := strings.TrimSpace(area)
	if needle == "" || len(signals) == 0 {
		return 0, "", false
	}
	lower := strings.ToLower(needle)
	best := 0
	bestAreas := ""
	for _, ws := range signals {
		if ws.Signal <= 0 {
			continue
		}
		if strings.Contains(strings.ToLower(ws.AffectedAreas), lower) {
			if ws.Signal > best {
				best = ws.Signal
				bestAreas = ws.AffectedAreas
			}
		}
	}
	if best == 0 {
		return 0, "", false
	}
	return best, bestAreas, true
}
