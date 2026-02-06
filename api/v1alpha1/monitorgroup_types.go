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

// MonitorGroupSpec defines desired state of MonitorGroup.
type MonitorGroupSpec struct {
	// SyncInterval defines how often the operator reconciles with the UptimeRobot API.
	//+kubebuilder:default:="24h"
	SyncInterval *metav1.Duration `json:"syncInterval,omitempty"`

	// Prune enables garbage collection.
	//+kubebuilder:default:=true
	Prune bool `json:"prune"`

	// Account references this object's Account. If not specified, the default will be used.
	Account corev1.LocalObjectReference `json:"account,omitempty"`

	// FriendlyName is the display name for the monitor group.
	//+kubebuilder:validation:MaxLength=255
	//+kubebuilder:validation:MinLength=1
	FriendlyName string `json:"friendlyName"`

	// Monitors references Monitor resources to include in this group.
	//+optional
	Monitors []corev1.LocalObjectReference `json:"monitors,omitempty"`

	// PullFromGroups specifies existing group IDs to pull monitors from.
	//+optional
	PullFromGroups []int `json:"pullFromGroups,omitempty"`
}

// MonitorGroupStatus defines observed state of MonitorGroup.
type MonitorGroupStatus struct {
	Ready          bool         `json:"ready"`
	ID             string       `json:"id,omitempty"`
	MonitorCount   int          `json:"monitorCount,omitempty"`
	LastReconciled *metav1.Time `json:"lastReconciled,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
//+kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.friendlyName"
//+kubebuilder:printcolumn:name="Count",type="integer",JSONPath=".status.monitorCount"
//+kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id",priority=1
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:resource:shortName=mgrp

// MonitorGroup is the Schema for the monitorgroups API.
type MonitorGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MonitorGroupSpec   `json:"spec,omitempty"`
	Status MonitorGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MonitorGroupList contains a list of MonitorGroup.
type MonitorGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MonitorGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MonitorGroup{}, &MonitorGroupList{})
}
