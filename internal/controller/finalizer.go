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
	"context"
	"fmt"
	"math"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// SkipCleanupAnnotation allows users to force-skip cleanup if API is permanently down
	SkipCleanupAnnotation = "uptimerobot.com/skip-cleanup"
	// CleanupStartTimeAnnotation tracks when cleanup first started
	CleanupStartTimeAnnotation = "uptimerobot.com/cleanup-start-time"
	// DefaultCleanupTimeout is the maximum duration to attempt cleanup before forcing finalizer removal
	DefaultCleanupTimeout = 10 * time.Minute
	// backoffExponent controls how aggressively backoff increases.
	// With exponent=3, backoff grows as: 30s * 2^(0*3), 30s * 2^(0.1*3), ..., reaching 240s at ~25% elapsed.
	// This produces the progression: 30s, 60s, 120s, 240s, capped at 5m.
	backoffExponent = 3
	// maxBackoffShift prevents integer overflow in backoff calculation.
	// 1 << 10 = 1024, giving max multiplier of ~1024 before capping at maxBackoff.
	maxBackoffShift = 10
)

// Standard deletion condition reasons
const (
	// ReasonCleanupStarted indicates cleanup has started
	ReasonCleanupStarted = "CleanupStarted"
	// ReasonCleanupInProgress indicates cleanup is in progress with retries
	ReasonCleanupInProgress = "CleanupInProgress"
	// ReasonCleanupSuccess indicates cleanup completed successfully
	ReasonCleanupSuccess = "CleanupSuccess"
	// ReasonCleanupSkipped indicates cleanup was skipped due to annotation
	ReasonCleanupSkipped = "CleanupSkipped"
	// ReasonCleanupTimeout indicates cleanup timed out and finalizer was force-removed
	ReasonCleanupTimeout = "CleanupTimeout"
	// ReasonCleanupError indicates cleanup failed with an error
	ReasonCleanupError = "CleanupError"
)

// CleanupResult represents the outcome of a cleanup attempt
type CleanupResult struct {
	// Success indicates if cleanup completed successfully
	Success bool
	// ForceRemove indicates the finalizer should be removed regardless of success
	ForceRemove bool
	// RequeueAfter is the duration to wait before retrying (if not successful and not force-removed)
	RequeueAfter time.Duration
	// Message is a human-readable status message
	Message string
}

// CleanupFunc is a function that performs the actual cleanup operation
type CleanupFunc func(ctx context.Context) error

// CleanupOptions configures cleanup behavior
type CleanupOptions struct {
	// Object is the resource being deleted
	Object client.Object
	// Conditions is the status conditions array to update
	Conditions *[]metav1.Condition
	// ObservedGeneration is the current observed generation
	ObservedGeneration int64
	// Recorder is used to emit events
	Recorder record.EventRecorder
	// CleanupTimeout is the maximum duration to attempt cleanup
	CleanupTimeout time.Duration
	// CleanupFunc performs the actual cleanup operation
	CleanupFunc CleanupFunc
}

// HandleFinalizerCleanup implements retry and timeout logic for finalizer cleanup
func HandleFinalizerCleanup(ctx context.Context, opts CleanupOptions) (CleanupResult, error) {
	// Use default timeout if not specified
	if opts.CleanupTimeout == 0 {
		opts.CleanupTimeout = DefaultCleanupTimeout
	}

	// Check for force-skip annotation
	if skipValue, ok := opts.Object.GetAnnotations()[SkipCleanupAnnotation]; ok && skipValue == "true" {
		msg := "Cleanup skipped due to skip-cleanup annotation"
		SetCondition(opts.Conditions, TypeDeleting, metav1.ConditionTrue, ReasonCleanupSkipped, msg, opts.ObservedGeneration)
		if opts.Recorder != nil {
			opts.Recorder.Event(opts.Object, "Warning", ReasonCleanupSkipped, msg)
		}
		return CleanupResult{
			Success:     true,
			ForceRemove: true,
			Message:     msg,
		}, nil
	}

	// Get or initialize cleanup start time
	annotations := opts.Object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	startTimeStr, exists := annotations[CleanupStartTimeAnnotation]
	var startTime time.Time
	if exists {
		var err error
		startTime, err = time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			// Invalid format, reset to now
			startTime = time.Now()
		}
	} else {
		startTime = time.Now()
	}

	// Calculate time elapsed since cleanup started
	elapsed := time.Since(startTime)

	// Check if we've exceeded the timeout
	if elapsed > opts.CleanupTimeout {
		msg := fmt.Sprintf("Cleanup timed out after %v, force-removing finalizer", opts.CleanupTimeout)
		SetCondition(opts.Conditions, TypeDeleting, metav1.ConditionTrue, ReasonCleanupTimeout, msg, opts.ObservedGeneration)
		if opts.Recorder != nil {
			opts.Recorder.Event(opts.Object, "Warning", ReasonCleanupTimeout, msg)
		}
		return CleanupResult{
			Success:     false,
			ForceRemove: true,
			Message:     msg,
		}, nil
	}

	// Set cleanup in progress condition
	msg := fmt.Sprintf("Cleanup in progress (elapsed: %v, timeout: %v)", elapsed.Round(time.Second), opts.CleanupTimeout)
	SetCondition(opts.Conditions, TypeDeleting, metav1.ConditionTrue, ReasonCleanupInProgress, msg, opts.ObservedGeneration)

	// Attempt cleanup
	err := opts.CleanupFunc(ctx)
	if err != nil {
		// Calculate backoff for retry
		// Use exponential backoff: min(30s * 2^shift, 5m)
		// where shift is based on how much time has elapsed relative to the total timeout.
		// Round() ensures proper progression from the first retry:
		// - At 5% elapsed (30s/10m): Round(0.05*3) = 0, gives 30s * 2^0 = 30s
		// - At 17% elapsed (102s/10m): Round(0.17*3) = 1, gives 30s * 2^1 = 60s
		// - At 34% elapsed (204s/10m): Round(0.34*3) = 1, gives 30s * 2^1 = 60s
		// - At 50% elapsed (5m/10m): Round(0.50*3) = 2, gives 30s * 2^2 = 120s
		backoffFactor := float64(elapsed) / float64(opts.CleanupTimeout)
		shift := int(math.Round(backoffFactor * backoffExponent))
		if shift > maxBackoffShift {
			shift = maxBackoffShift
		}
		backoff := 30 * time.Second * time.Duration(1<<shift)
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}

		errorMsg := fmt.Sprintf("Cleanup failed: %v (will retry in %v)", err, backoff.Round(time.Second))
		SetCondition(opts.Conditions, TypeDeleting, metav1.ConditionTrue, ReasonCleanupError, errorMsg, opts.ObservedGeneration)
		if opts.Recorder != nil {
			opts.Recorder.Event(opts.Object, "Warning", ReasonCleanupError, errorMsg)
		}

		return CleanupResult{
			Success:      false,
			ForceRemove:  false,
			RequeueAfter: backoff,
			Message:      errorMsg,
		}, err
	}

	// Cleanup succeeded
	msg = "Cleanup completed successfully"
	SetCondition(opts.Conditions, TypeDeleting, metav1.ConditionTrue, ReasonCleanupSuccess, msg, opts.ObservedGeneration)
	if opts.Recorder != nil {
		opts.Recorder.Event(opts.Object, "Normal", ReasonCleanupSuccess, msg)
	}

	return CleanupResult{
		Success:     true,
		ForceRemove: false,
		Message:     msg,
	}, nil
}

// InitializeCleanupTracking sets the cleanup start time annotation if not already set
func InitializeCleanupTracking(ctx context.Context, k8sClient client.Client, obj client.Object) error {
	annotations := obj.GetAnnotations()
	if _, exists := annotations[CleanupStartTimeAnnotation]; exists {
		return nil
	}

	key := client.ObjectKeyFromObject(obj)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := obj.DeepCopyObject().(client.Object)
		if err := k8sClient.Get(ctx, key, current); err != nil {
			return err
		}

		currentAnnotations := current.GetAnnotations()
		if currentAnnotations == nil {
			currentAnnotations = make(map[string]string)
		}
		if _, exists := currentAnnotations[CleanupStartTimeAnnotation]; exists {
			obj.SetAnnotations(currentAnnotations)
			return nil
		}

		currentAnnotations[CleanupStartTimeAnnotation] = time.Now().Format(time.RFC3339)
		current.SetAnnotations(currentAnnotations)
		if err := k8sClient.Update(ctx, current); err != nil {
			return err
		}

		obj.SetAnnotations(current.GetAnnotations())
		obj.SetResourceVersion(current.GetResourceVersion())
		return nil
	})
}

// SetDeletingCondition sets the Deleting condition
func SetDeletingCondition(conditions *[]metav1.Condition, reason, message string, observedGeneration int64) {
	SetCondition(conditions, TypeDeleting, metav1.ConditionTrue, reason, message, observedGeneration)
}
