// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ph-commons/pagasa-pp-cli/internal/client"
)

func TestWriteAPIErrorEnvelope_OmitsBody(t *testing.T) {
	// Capture stdout envelope written for --json machine path.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	apiErr := &client.APIError{
		Method:     "GET",
		Path:       "https://example.test/weather",
		StatusCode: 503,
		Body:       "<html>" + strings.Repeat("challenge ", 500) + "</html>",
	}
	writeAPIErrorEnvelope(&rootFlags{asJSON: true}, apiErr, 5)

	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()

	out := buf.String()
	if strings.Contains(out, "challenge") || strings.Contains(out, "<html>") {
		t.Fatalf("machine envelope leaked body: %s", out)
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if payload["code"].(float64) != 5 {
		t.Fatalf("code = %v", payload["code"])
	}
	if int(payload["status"].(float64)) != 503 {
		t.Fatalf("status = %v", payload["status"])
	}
	errStr, _ := payload["error"].(string)
	if !strings.Contains(errStr, "HTTP 503") {
		t.Fatalf("error field = %q", errStr)
	}
	if strings.Contains(errStr, "challenge") {
		t.Fatalf("error field leaked body: %q", errStr)
	}
}
