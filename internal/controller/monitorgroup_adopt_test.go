/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
)

func TestFindAdoptableGroup(t *testing.T) {
	groups := []uptimerobot.GroupWireFormat{
		{ID: 5, Name: "alpha"},
		{ID: 3, Name: "beta"},
		{ID: 9, Name: "beta"},
		{ID: 7, Name: "gamma"},
		{ID: 0, Name: "zero"},
	}

	tests := []struct {
		name           string
		friendlyName   string
		groups         []uptimerobot.GroupWireFormat
		wantID         int
		wantMatchCount int
	}{
		{
			name:           "empty friendly name never matches",
			friendlyName:   "",
			groups:         groups,
			wantMatchCount: 0,
		},
		{
			name:           "no matching name",
			friendlyName:   "delta",
			groups:         groups,
			wantMatchCount: 0,
		},
		{
			name:           "single match returns that group",
			friendlyName:   "alpha",
			groups:         groups,
			wantID:         5,
			wantMatchCount: 1,
		},
		{
			name:           "multiple matches return lowest ID for deterministic adoption",
			friendlyName:   "beta",
			groups:         groups,
			wantID:         3,
			wantMatchCount: 2,
		},
		{
			name:           "zero-ID group is treated as a real match (no zero-value sentinel collision)",
			friendlyName:   "zero",
			groups:         groups,
			wantID:         0,
			wantMatchCount: 1,
		},
		{
			name:           "empty group list does not match",
			friendlyName:   "alpha",
			groups:         nil,
			wantMatchCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, matchCount := findAdoptableGroup(tc.groups, tc.friendlyName)
			if matchCount != tc.wantMatchCount {
				t.Fatalf("matchCount = %d, want %d", matchCount, tc.wantMatchCount)
			}
			if matchCount > 0 && got.ID != tc.wantID {
				t.Fatalf("got ID %d, want %d", got.ID, tc.wantID)
			}
		})
	}
}
