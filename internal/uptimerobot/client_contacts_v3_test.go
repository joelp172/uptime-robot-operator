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

package uptimerobot

import (
	"testing"
	"time"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestContactsToV3Format_ThresholdAndRecurrenceInMinutes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		threshold      time.Duration
		recurrence     time.Duration
		wantThreshold  int
		wantRecurrence int
	}{
		{
			name:           "1m default threshold sends as 1 minute (not 60)",
			threshold:      1 * time.Minute,
			recurrence:     0,
			wantThreshold:  1,
			wantRecurrence: 0,
		},
		{
			name:           "5m threshold, 15m recurrence",
			threshold:      5 * time.Minute,
			recurrence:     15 * time.Minute,
			wantThreshold:  5,
			wantRecurrence: 15,
		},
		{
			name:           "zero threshold",
			threshold:      0,
			recurrence:     0,
			wantThreshold:  0,
			wantRecurrence: 0,
		},
		{
			name:           "sub-minute threshold rounds to nearest minute (UptimeRobot granularity)",
			threshold:      29 * time.Second, // <30s → rounds to 0
			recurrence:     0,
			wantThreshold:  0,
			wantRecurrence: 0,
		},
		{
			name:           "sub-minute threshold at 31s rounds up to 1 minute",
			threshold:      31 * time.Second,
			recurrence:     0,
			wantThreshold:  1,
			wantRecurrence: 0,
		},
		{
			name:           "large threshold preserved",
			threshold:      60 * time.Minute,
			recurrence:     30 * time.Minute,
			wantThreshold:  60,
			wantRecurrence: 30,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			contacts := uptimerobotv1.MonitorContacts{
				{
					ID: "123",
					MonitorContactCommon: uptimerobotv1.MonitorContactCommon{
						Threshold:  metav1.Duration{Duration: tc.threshold},
						Recurrence: metav1.Duration{Duration: tc.recurrence},
					},
				},
			}

			got := contactsToV3Format(contacts)
			if len(got) != 1 {
				t.Fatalf("expected one entry, got %d", len(got))
			}
			if got[0].Threshold != tc.wantThreshold {
				t.Errorf("threshold: got %d (minutes) want %d", got[0].Threshold, tc.wantThreshold)
			}
			if got[0].Recurrence != tc.wantRecurrence {
				t.Errorf("recurrence: got %d (minutes) want %d", got[0].Recurrence, tc.wantRecurrence)
			}
		})
	}
}

func TestContactsToV3Format_NegativeThresholdClamped(t *testing.T) {
	t.Parallel()

	contacts := uptimerobotv1.MonitorContacts{
		{
			ID: "123",
			MonitorContactCommon: uptimerobotv1.MonitorContactCommon{
				Threshold: metav1.Duration{Duration: -5 * time.Minute},
			},
		},
	}

	got := contactsToV3Format(contacts)
	if len(got) != 1 || got[0].Threshold != 0 {
		t.Fatalf("expected threshold clamped to 0, got %+v", got)
	}
}

func TestContactsToV3Format_SkipsEntriesWithoutID(t *testing.T) {
	t.Parallel()

	contacts := uptimerobotv1.MonitorContacts{
		{ID: ""},
		{ID: "42", MonitorContactCommon: uptimerobotv1.MonitorContactCommon{
			Threshold: metav1.Duration{Duration: 3 * time.Minute},
		}},
	}

	got := contactsToV3Format(contacts)
	if len(got) != 1 || got[0].AlertContactID != "42" || got[0].Threshold != 3 {
		t.Fatalf("unexpected result: %+v", got)
	}
}
