// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/ph-commons/pagasa-pp-cli/internal/client"
)

// pageFetch is one path to retrieve in parallel.
type pageFetch struct {
	Path string
	// Required marks paths whose failure aborts the multi-fetch (e.g. synopsis
	// for now/approach). Optional paths record nil body + err for best-effort
	// composition (digest city forecast, bulletin when synopsis is primary).
	Required bool
}

// pageBody is the result of one pageFetch.
type pageBody struct {
	Path string
	Body json.RawMessage
	Err  error
}

// fetchPages retrieves independent HTML pages concurrently. PAGASA novel
// commands often need 2–3 public pages; sequential GETs pay full RTT per
// page. Results are returned in the same order as paths. Shared *client.Client
// is safe for concurrent Get: rate limiter is mutexed; lastContentType races
// only affect optional cache content-type sidecars (always text/html here).
func fetchPages(ctx context.Context, c *client.Client, paths []pageFetch) []pageBody {
	out := make([]pageBody, len(paths))
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		go func(i int, p pageFetch) {
			defer wg.Done()
			body, err := c.Get(ctx, p.Path, nil)
			out[i] = pageBody{Path: p.Path, Body: body, Err: err}
		}(i, p)
	}
	wg.Wait()
	return out
}

// firstRequiredError returns the first required path's error, or nil.
func firstRequiredError(paths []pageFetch, bodies []pageBody) error {
	for i, p := range paths {
		if p.Required && bodies[i].Err != nil {
			return bodies[i].Err
		}
	}
	return nil
}
