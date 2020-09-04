package v1

import (
	corev1 "k8s.io/api/core/v1"
)

// +k8s:deepcopy-gen=true
// ReplicaSpec is a description of the job replica.
type ReplicaSpec struct {
	MinReplicas     *int32                   `json:"minReplicas,omitempty"`
	MaxReplicas     *int32                   `json:"maxReplicas,omitempty"`
	Replicas        *int32                   `json:"replicas,omitempty"`
	RestartLimit    *int32                   `json:"restartLimit,omitempty"`
	Template        corev1.PodTemplateSpec   `json:"template,omitempty"`
	RestartPolicy   RestartPolicy            `json:"restartPolicy,omitempty"`
	RestartScope    RestartScope             `json:"restartScope,omitempty"`
	FailPolicy      EndingPolicy             `json:"failPolicy,omitempty"`
	CompletePolicy  EndingPolicy             `json:"completePolicy,omitempty"`
	EdlPolicy       EdlPolicy                `json:"edlPolicy,omitempty"`
}

type RestartPolicy string
type RestartScope string
const (
	RestartPolicyAlways     RestartPolicy = "Always"
	RestartPolicyOnFailure  RestartPolicy = "OnFailure"
	RestartPolicyOnNodeFail RestartPolicy = "OnNodeFail"
	RestartPolicyNever      RestartPolicy = "Never"
	RestartPolicyExitCode   RestartPolicy = "ExitCode"
	RestartPolicyOnNodeFailWithExitCode RestartPolicy = "OnNodeFailWithExitCode"
	RestartScopeAll         RestartScope = "All"
	RestartScopeReplica     RestartScope = "Replica"
	RestartScopePod         RestartScope = "Pod"
)

type ReplicaStatus struct {
	// The number of pending pods.
	Pending int32 `json:"pending,omitempty"`
	// The number of scheduled pods.
	Scheduled int32 `json:"scheduled,omitempty"`
	// The number of actively running pods.
	Active int32 `json:"active,omitempty"`
	// The number of pods which reached phase Succeeded.
	Succeeded int32 `json:"succeeded,omitempty"`
	// The number of restarting pods.
	Restarting int32 `json:"restarting,omitempty"`
	// The number of pods which reached phase Failed.
	Failed int32 `json:"failed,omitempty"`
}

type EdlPolicy string
const (
	EdlPolicyAuto        EdlPolicy = "Auto"
	EdlPolicyManual      EdlPolicy = "Manual"
	EdlPolicyNever       EdlPolicy = "Never"
)
type EndingPolicy string 
const (
	EndingPolicyAll EndingPolicy = "All"
	EndingPolicyRank0 EndingPolicy = "Rank0"
	EndingPolicyAny EndingPolicy = "Any"
	EndingPolicyNone EndingPolicy = "None"
)
