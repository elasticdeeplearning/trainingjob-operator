package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

func Int32(v int32) *int32 {
	return &v
}

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

func setDefaultReplicas(spec *ReplicaSpec) {
	if spec.Replicas == nil {
		spec.Replicas = Int32(1)
	}
	if spec.RestartPolicy == "" {
		spec.RestartPolicy = RestartPolicyNever
	}
	if spec.RestartScope == "" {
		spec.RestartScope = RestartScopeAll
	}
	if spec.FailPolicy == "" {
		spec.FailPolicy = EndingPolicyAny
	}
	if spec.CompletePolicy == "" {
		spec.CompletePolicy = EndingPolicyAll
	}
}

// SetDefaults_TrainingJob sets any unspecified values to defaults.
func SetDefaults_AITrainingJob(job *AITrainingJob) {
	// Set default cleanpod policy to All
	if job.Spec.CleanPodPolicy == nil {
		all := CleanPodPolicyAll
		job.Spec.CleanPodPolicy = &all
	}
	// Set default failed policy to EndingPolicyAny
	if job.Spec.FailPolicy == "" {
		job.Spec.FailPolicy = EndingPolicyAny
	}
	// Set default completed policy to EndingPolicyAll.
	if job.Spec.CompletePolicy == "" {
		job.Spec.CompletePolicy = EndingPolicyAll
	}

	for _, spec := range job.Spec.ReplicaSpecs {
		// Set default replicas to 1.
		setDefaultReplicas(spec)
	}
}
