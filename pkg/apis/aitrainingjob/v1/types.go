/*
Copyright (c) 2020 PaddlePaddle Authors. All Rights Reserved.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=aitrainingjob

// AITrainingJob is a specification for a AITrainingJob resource
type AITrainingJob struct {
	metav1.TypeMeta `json:",inline"`
	// ObjectMeta is metadata that all persisted resources must have, which includes all objects
	// users must create.
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the specification of the desired behavior of the TrainingJob.
	Spec TrainingJobSpec `json:"spec,omitempty"`
	// Status is the most recently observed status of the TrainingJob.
	Status TrainingJobStatus `json:"status,omitempty"`
}

// TrainingJobSpec is the spec for a AITrainingJob resource
type TrainingJobSpec struct {
    // Define restarting exitcode
    RestartingExitCode RestartingExitCode  `json:"restartingExitCode,omitempty"`
	// Specify the framework, eg: tensorflow / paddlepaddle
	FrameworkType FrameworkType `json:"frameworkType,omitempty"`
	// Identify whether fault tolerance is required
	FaultTolerant bool `json:"faultTolerant,omitempty"`
	// Specify the job priority
	Priority string `json:"priority,omitempty"`
	// Specify the Kubernetes scheduler name
	SchedulerName string `json:"schedulerName,omitempty"`
	// Time limit for the job (in seconds)
	TimeLimit *int64 `json:"timeLimit,omitempty"`
	// Define the policy for cleaning up pods after the trainingjob completes
	CleanPodPolicy *CleanPodPolicy `json:"cleanPodPolicy,omitempty"`
	// Define the policy for fail
	FailPolicy EndingPolicy `json:"failPolicy,omitempty"`
	// Define the policy for complete
	CompletePolicy EndingPolicy `json:"completePolicy,omitempty"`
	// Specify the TrainingJob configuration
	ReplicaSpecs map[ReplicaName]*ReplicaSpec `json:"replicaSpecs"`
}

type CleanPodPolicy string
type RestartingExitCode string

const (
	// Delete all pods/services, when job is finished
	CleanPodPolicyAll CleanPodPolicy = "All"
	// Delete nothing
	CleanPodPolicyNone CleanPodPolicy = "None"
)

// +k8s:deepcopy-gen=true
// TrainingJobStatus is the status for a AITrainingJob resource
type TrainingJobStatus struct {
	// The phase of a job is a simple, high-level summary of where the Job is in its lifecycle.
	Phase TrainingJobPhase `json:"phase"`
	// An array of current job conditions
	Conditions []TrainingJobCondition `json:"conditions"`
	// detail status of echo replica resource
	ReplicaStatuses map[ReplicaName]*ReplicaStatus `json:"replicaStatuses"`
	// The times of pods restart.
	RestartCountes map[ReplicaName]int32 `json:"RestartCount,,omitempty"`
	// ReplicaName need to restart
	RestartReplicaName ReplicaName 
	// Represents the time when the job was acknowledged by the job controller
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// Represents the time when the job start running.
	StartRunningTime *metav1.Time `json:"startRunningTime,omitempty"`
	// Represents the time when the job was completed
	EndTime *metav1.Time `json:"endTime,omitempty"`
	// Represents the last time when the job was reconciled.
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// TrainingJobPhase is the phase of AITrainingJob
type TrainingJobPhase string

const (
	// None means the job has been accepted by the system
	TrainingJobPhaseNone TrainingJobPhase = ""
	// Pending means one or more of the pods/services has not been scheduled.
	TrainingJobPhasePending = "Pending"
	// Creating means all pods/services of this job have been successfully scheduled,
	// but one or more of the pods/services has not been launched.
	TrainingJobPhaseCreating = "Creating"
	// Running means all pods/services have been launched.
	TrainingJobPhaseRunning = "Running"
	// Succeed means all pods of this job reached phase success.
	TrainingJobPhaseSucceeded = "Succeed"
	// Failed means one or more pods of this job reached phase failed.
	TrainingJobPhaseFailed = "Failed"
	// Timeout means the job runs over the preset maximum run time
	TrainingJobPhaseTimeout = "Timeout"
	// TODO: Restarting means the job is restarting
	TrainingJobPhaseRestarting = "Restarting"
	// Terminating means the job have been terminated, but resources are not yet fully released
	TrainingJobPhaseTerminating = "Terminating"
	// Preempted means the job have been preempted, and resources were fully released
	TrainingJobPhasePreempted = "Preempted"
	// NodeFail means the node is failed
	TrainingJobPhaseNodeFail = "NodeFail"
)

// +k8s:deepcopy-gen=true
// TrainingJobCondition describes the state of the job at a certain point.
type TrainingJobCondition struct {
	// Type is the type of the condition.
	Type TrainingJobPhase `json:"type"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Unique, one-word, CamelCase reason for the condition's last transition
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition.
	Message string `json:"message,omitempty"`
	// Last time we probed the condition.
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=aitrainingjobs

// AITrainingJobList is a list of AITrainingJob resources
type AITrainingJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []AITrainingJob `json:"items"`
}
