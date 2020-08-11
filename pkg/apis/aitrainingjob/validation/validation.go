package validation

import (
	"fmt"
	trainingjobv1 "github.com/elasticdeeplearning/trainingjob-operator/pkg/apis/aitrainingjob/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ValidateV1TrainingJobSpec checks that the v1.Trainingjob is valid.
func ValidateV1TrainingJobSpec(job *trainingjobv1.Trainingjob) error {
	return validateV1ReplicaSpecs(job.ReplicaSpecs)
}

func validateV1ReplicaSpecs(specs map[trainingjobv1.ReplicaName]*trainingjobv1.ReplicaSpec) error {
	if specs == nil {
		return fmt.Errorf("Trainingjob is not valid")
	}
	for rType, value := range specs {
		if value == nil || len(value.Template.Spec.Containers) == 0 {
			return fmt.Errorf("Trainingjob is not valid: containers definition expected in %v", rType)
		}
		// Make sure the image is defined in the container.
		for _, container := range value.Template.Spec.Containers {
			if container.Image == "" {
				msg := fmt.Sprintf("Trainingjob is not valid: Image is undefined in the container of %v", rType)
				log.Error(msg)
				return fmt.Errorf(msg)
			}
		}
	}
	return nil
}
