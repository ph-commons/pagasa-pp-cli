// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
)

var issuedAtRe = regexp.MustCompile(`(?i)^At\s+([0-9: ]+[AP]M[^,]*),`)

type nowView struct {
	pagasa.Synopsis
	IssuedAt string `json:"issued_at,omitempty"`
	Source   string `json:"source"`
}

func newNowCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "now",
		Short:   "Current PAGASA weather synopsis and active cyclone, if any",
		Long:    "Fetch the live PAGASA synopsis paragraph from the public weather page and extract the active tropical cyclone name and issuance time when present.",
		Example: "  pagasa-pp-cli now --json",
		// No mcp:read-only: always best-effort saveSnapshot (local SQLite) after
		// live fetch. Not mcp:local-write (that forces openWorldHint=false).
		// mcp:open-world: outbound PAGASA HTTP (#23, #24).
		Annotations: map[string]string{"mcp:open-world": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch /weather synopsis")
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			raw, err := c.Get(ctx, "/weather", nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			syn, ok := pagasa.ParseSynopsis(string(raw))
			if !ok {
				return fmt.Errorf("could not locate the Synopsis panel on /weather; PAGASA may have restructured the page")
			}
			view := nowView{Synopsis: syn, Source: "live"}
			if im := issuedAtRe.FindStringSubmatch(syn.Text); im != nil {
				view.IssuedAt = strings.TrimSpace(im[1])
			}
			// Best-effort snapshot so `history` accumulates. Never fail a live read on a cache miss.
			if err := saveSnapshot(ctx, defaultDBPath("pagasa-pp-cli"), snapshot{
				Synopsis: syn.Text, StormName: syn.StormName, StormKind: syn.StormKind,
			}); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not persist snapshot: %v\n", err)
			}
			if machineOut(cmd, flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.StormName != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Active: %s %q\n\n", view.StormKind, view.StormName)
			}
			fmt.Fprintln(cmd.OutOrStdout(), view.Text)
			return nil
		},
	}
	return cmd
}

// machineOut reports whether output should be machine JSON (explicit --json/--agent or piped stdout).
func machineOut(cmd *cobra.Command, flags *rootFlags) bool {
	return flags.asJSON || flags.agent || !isTerminal(cmd.OutOrStdout())
}
