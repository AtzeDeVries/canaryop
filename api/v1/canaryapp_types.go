/*
Copyright 2021.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CanaryAppSpec defines the desired state of CanaryApp
type CanaryAppSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//+kubebuilder:validation:MinLength=0
	// Image to be deployed
	Image string `json:"image"`

	//+kubebuilder:validation:Minimum=1
	// Replicas of deployment
	Replicas int32 `json:"replicas"`

	//+kubebuilder:validation:Minimum=1
	// TestReplicas amount of replicas for smoke test
	TestReplicas int32 `json:"secondaryReplicas"`

	//+kubebuilder:validation:MinLength=0
	// Prometheus query to check state of deployment should return result if deployment is failed
	PrometheusQuery string `json:"prometheusQuery"`

	//+kubebuilder:validation:MinLength=0
	// Url of the prometheus server
	PrometheusURL string `json:"prometheusURL"`
}

// CanaryAppStatus defines the observed state of CanaryApp
type CanaryAppStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// A bool which shows if the update has been successful
	SuccessfulRelease bool `json:"successfulRelease"`
	// A bool which shows if a smoke test is running
	TestRunning bool `json:"testRunning"`
	// Percentage of traffic send to new version
	TrafficShift int32 `json:"trafficShift"`
	// States which tag failed on deployment so we don't redeploy
	LastFailedImage string `json:"lastFailedTag"`
	// When the last trafficshift shift was done
	LastTrafficShift *metav1.Time `json:"lastTrafficShift,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CanaryApp is the Schema for the canaryapps API
type CanaryApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CanaryAppSpec   `json:"spec,omitempty"`
	Status CanaryAppStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CanaryAppList contains a list of CanaryApp
type CanaryAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CanaryApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CanaryApp{}, &CanaryAppList{})
}
