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

// ContactSpec defines the desired state of Contact.
type ContactSpec struct {
	//+kubebuilder:default:=false
	IsDefault bool `json:"isDefault,omitempty"`

	// Account references this object's Account. If not specified, the default will be used.
	Account corev1.LocalObjectReference `json:"account,omitempty"`

	// Contact configures the Uptime Robot monitor.
	Contact ContactValues `json:"contact"`
}

// ContactStatus defines the observed state of Contact.
type ContactStatus struct {
	Ready bool   `json:"ready"`
	ID    string `json:"id,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
//+kubebuilder:printcolumn:name="Default",type="boolean",JSONPath=".spec.isDefault"
//+kubebuilder:printcolumn:name="Friendly Name",type="string",JSONPath=".spec.contact.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Contact is the Schema for the contacts API.
type Contact struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContactSpec   `json:"spec,omitempty"`
	Status ContactStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ContactList contains a list of Contact.
type ContactList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Contact `json:"items"`
}

type ContactValues struct {
	// ID is the UptimeRobot alert contact ID. Use this when the contact has no friendlyName set.
	// Get contact IDs via: curl -H "Authorization: Bearer YOUR_API_KEY" https://api.uptimerobot.com/v3/user/alert-contacts
	// Either ID or Name must be specified, but not both.
	// +optional
	ID string `json:"id,omitempty"`

	// Name matches the friendlyName of an existing alert contact in UptimeRobot.
	// Either ID or Name must be specified, but not both.
	// +optional
	Name string `json:"name,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Contact{}, &ContactList{})
}
