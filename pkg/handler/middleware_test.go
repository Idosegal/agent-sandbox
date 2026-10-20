/*
 * Copyright 2025 The https://github.com/agent-sandbox/agent-sandbox Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package handler

import (
	"bytes"
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func TestRedactHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("X-API-Key", "e2b_secret")
	h.Set("Authorization", "Bearer secret")
	h.Set("X-Access-Token", "secret")
	h.Set("Cookie", "session=secret")
	h.Set("Content-Type", "application/json")

	got := redactHeaders(h)

	for _, name := range []string{"X-Api-Key", "Authorization", "X-Access-Token", "Cookie"} {
		if v := got.Get(name); v != "[REDACTED]" {
			t.Errorf("%s = %q, want [REDACTED]", name, v)
		}
	}
	if v := got.Get("Content-Type"); v != "application/json" {
		t.Errorf("Content-Type = %q, want it unchanged", v)
	}
	if v := h.Get("X-Api-Key"); v != "e2b_secret" {
		t.Errorf("original header modified: X-Api-Key = %q", v)
	}
}

func TestLoggingMiddlewareDoesNotLogAPIKey(t *testing.T) {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	_ = fs.Set("v", "2")
	_ = fs.Set("logtostderr", "false")
	_ = fs.Set("alsologtostderr", "false")
	var buf bytes.Buffer
	klog.SetOutput(&buf)
	defer klog.SetOutput(nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "e2b_secret" {
			t.Errorf("handler saw X-Api-Key %q, want the original value", r.Header.Get("X-Api-Key"))
		}
		w.Header().Set("Set-Cookie", "session=secret")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sandbox", nil)
	req.Header.Set("X-API-Key", "e2b_secret")

	LoggingMiddleware(next).ServeHTTP(httptest.NewRecorder(), req)
	klog.Flush()

	out := buf.String()
	if !strings.Contains(out, "Started request") {
		t.Fatalf("request was not logged at -v=2: %q", out)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("log contains a credential: %q", out)
	}
}
