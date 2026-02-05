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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:generate=true
//+kubebuilder:validation:XValidation:rule="self.interval == 'once' || self.interval == 'daily' || (has(self.days) && size(self.days) > 0)", message="days field is required and must not be empty when interval is weekly or monthly"
//+kubebuilder:validation:XValidation:rule="!has(self.days) || self.interval == 'weekly' || self.interval == 'monthly'", message="days field is only valid for weekly or monthly intervals"
//+kubebuilder:validation:XValidation:rule="!has(self.days) || self.interval != 'weekly' || self.days.all(d, d >= 0 && d <= 6)", message="days must be 0-6 for weekly interval (0=Sunday)"
//+kubebuilder:validation:XValidation:rule="!has(self.days) || self.interval != 'monthly' || self.days.all(d, (d >= 1 && d <= 31) || d == -1)", message="days must be 1-31 or -1 (last day) for monthly interval"
//+kubebuilder:validation:XValidation:rule="!has(self.startDate) || self.interval == 'once'", message="startDate is only valid for once interval"
//+kubebuilder:validation:XValidation:rule="self.interval != 'once' || has(self.startDate)", message="startDate is required for once interval"

// MaintenanceWindowSpec defines the desired state of MaintenanceWindow.
type MaintenanceWindowSpec struct {
	// SyncInterval defines how often the operator reconciles with the UptimeRobot API.
	// This controls drift detection frequency.
	//+kubebuilder:default:="24h"
	SyncInterval *metav1.Duration `json:"syncInterval,omitempty"`

	// Prune enables garbage collection.
	//+kubebuilder:default:=true
	Prune bool `json:"prune,omitempty"`

	// Account references this object's Account. If not specified, the default will be used.
	Account corev1.LocalObjectReference `json:"account,omitempty"`

	// Name is the friendly name of the maintenance window (max 255 chars).
	Name string `json:"name"`

	// Interval defines the recurrence pattern of the maintenance window.
	//+kubebuilder:validation:Enum=once;daily;weekly;monthly
	Interval string `json:"interval"`

	// StartDate is the start date of the maintenance window in YYYY-MM-DD format.
	// Required for once interval, not allowed for daily/weekly/monthly intervals.
	//+optional
	//+kubebuilder:validation:Pattern=`^(19|20)\d{2}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$`
	StartDate string `json:"startDate,omitempty"`

	// StartTime is the start time of the maintenance window in HH:mm:ss format.
	//+kubebuilder:validation:Pattern=`^(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d$`
	StartTime string `json:"startTime"`

	// Duration is the duration of the maintenance window.
	// Supports Go duration format (e.g., "30m", "1h", "2h30m").
	// Minimum value is 1 minute.
	Duration metav1.Duration `json:"duration"`

	// Days specifies which days the maintenance window runs on.
	// For weekly: day of week (0=Sunday, 1=Monday, ..., 6=Saturday).
	// For monthly: day of month (1-31, -1 for last day of month).
	// Required for weekly and monthly intervals.
	//+optional
	//+kubebuilder:validation:MaxItems=31
	Days []int `json:"days,omitempty"`

	// AutoAddMonitors, when true, automatically adds all monitors to this maintenance window.
	//+optional
	AutoAddMonitors bool `json:"autoAddMonitors,omitempty"`

	// MonitorRefs is a list of Monitor resources to add to this maintenance window.
	// Each reference specifies the monitor name and is resolved within the same namespace
	// as this MaintenanceWindow.
	//+optional
	MonitorRefs []corev1.LocalObjectReference `json:"monitorRefs,omitempty"`
}

// MaintenanceWindowStatus defines the observed state of MaintenanceWindow.
type MaintenanceWindowStatus struct {
	// Ready indicates if the maintenance window is successfully created in UptimeRobot.
	Ready bool `json:"ready"`

	// ID is the UptimeRobot maintenance window ID.
	ID string `json:"id,omitempty"`

	// MonitorCount is the number of monitors assigned to this maintenance window.
	MonitorCount int `json:"monitorCount,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
//+kubebuilder:printcolumn:name="Interval",type="string",JSONPath=".spec.interval"
//+kubebuilder:printcolumn:name="Start Date",type="string",JSONPath=".spec.startDate"
//+kubebuilder:printcolumn:name="Duration",type="string",JSONPath=".spec.duration"
//+kubebuilder:printcolumn:name="Monitor Count",type="integer",JSONPath=".status.monitorCount"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// MaintenanceWindow is the Schema for the maintenancewindows API.
type MaintenanceWindow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaintenanceWindowSpec   `json:"spec,omitempty"`
	Status MaintenanceWindowStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MaintenanceWindowList contains a list of MaintenanceWindow.
type MaintenanceWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaintenanceWindow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaintenanceWindow{}, &MaintenanceWindowList{})
}
