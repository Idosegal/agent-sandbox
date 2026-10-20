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


package config

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func TestCheckFeatureDoesNotLogAPIKeys(t *testing.T) {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	_ = fs.Set("logtostderr", "false")
	_ = fs.Set("alsologtostderr", "false")
	var buf bytes.Buffer
	klog.SetOutput(&buf)
	defer klog.SetOutput(nil)

	prev := Cfg
	defer func() { Cfg = prev }()
	Cfg = &Config{FeatureConfig: FeatureConfig{ExperimentTokens: "other-0123456789abcdef,user-fedcba9876543210"}}

	if !CheckFeature(true, "user-fedcba9876543210") {
		t.Errorf("CheckFeature = false, want true for a listed user")
	}
	klog.Flush()

	out := buf.String()
	for _, key := range []string{"other-0123456789abcdef", "user-fedcba9876543210"} {
		if strings.Contains(out, key) {
			t.Errorf("log contains the full key %q: %q", key, out)
		}
	}
}
