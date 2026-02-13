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
	"context"
	"testing"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/uptimerobottest"
)

func TestPauseMonitor(t *testing.T) {
	srv := uptimerobottest.NewServer()
	defer srv.Close()

	t.Setenv("UPTIME_ROBOT_API", srv.URL)

	client := NewClient("test-api-key")
	ctx := context.Background()

	t.Run("successfully pauses a monitor", func(t *testing.T) {
		err := client.PauseMonitor(ctx, "12345")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("is idempotent - pausing already paused monitor succeeds", func(t *testing.T) {
		err := client.PauseMonitor(ctx, "12345")
		if err != nil {
			t.Errorf("expected no error on second pause, got %v", err)
		}
	})
}

func TestStartMonitor(t *testing.T) {
	srv := uptimerobottest.NewServer()
	defer srv.Close()

	t.Setenv("UPTIME_ROBOT_API", srv.URL)

	client := NewClient("test-api-key")
	ctx := context.Background()

	t.Run("successfully starts a monitor", func(t *testing.T) {
		err := client.StartMonitor(ctx, "12345")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("is idempotent - starting already active monitor succeeds", func(t *testing.T) {
		err := client.StartMonitor(ctx, "12345")
		if err != nil {
			t.Errorf("expected no error on second start, got %v", err)
		}
	})
}
