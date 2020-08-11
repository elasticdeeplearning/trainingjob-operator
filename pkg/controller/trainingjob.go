package controller

import (
	"strings"

	trainingjobv1 "github.com/elasticdeeplearning/trainingjob-operator/pkg/apis/aitrainingjob/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func (tc *TrainingJobController) GenGeneralName(jobName, rtype, index string) string {
	n := jobName + "-" + rtype + "-" + index
	return strings.Replace(n, "/", "-", -1)
}

func (tc *TrainingJobController) addTrainingJob(obj interface{}) {
	klog.V(4).Infof("AddFunc called")
	job := obj.(*trainingjobv1.AITrainingJob)
	klog.V(2).Infof("Informer: Add TrainingJob %s/%s.", job.Namespace, job.Name)
	// FIXME: need to validate trainingjob

	tc.enqueueJob(job, false, 0)
}

func (tc *TrainingJobController) updateTrainingJob(old, cur interface{}) {
	oldTJ := old.(*trainingjobv1.AITrainingJob)
	newTJ := cur.(*trainingjobv1.AITrainingJob)
	if oldTJ.ResourceVersion == newTJ.ResourceVersion {
		klog.V(2).Infof("Same Resourceversion for training job %s/%s, skipped", oldTJ.Namespace, oldTJ.Name)
		return
	}
	// FIXME: need to validate trainingjob

	klog.V(2).Infof("Informer: Update TrainingJob %s/%s.", oldTJ.Namespace, oldTJ.Name)
	tc.enqueueJob(newTJ, true, 0)

	if newTJ.Status.StartRunningTime != nil && newTJ.Spec.TimeLimit != nil &&
		(oldTJ.Spec.TimeLimit == nil || *oldTJ.Spec.TimeLimit != *newTJ.Spec.TimeLimit) {
		now := metav1.Now().Unix()
		passed := now - newTJ.Status.StartRunningTime.Unix()
		total := *newTJ.Spec.TimeLimit
		tc.enqueueJob(newTJ, false, int64(total-passed))
		klog.Infof("job TimeLimit updated, will rsync after %v seconds", int64(total-passed))
	}

}

func (tc *TrainingJobController) deleteTrainingJob(obj interface{}) {
	tc.enqueueJob(obj, false, 0)
}

func (tc *TrainingJobController) deletePodsAndServices(
	job *trainingjobv1.AITrainingJob,
	pods []*corev1.Pod,
	services []*corev1.Service) error {
	if len(pods) == 0 {
		return nil
	}
	// Delete pods
	for _, pod := range pods {
		if err := tc.podControl.DeletePod(pod.Namespace, pod.Name, job); err != nil {
			return err
		}
	}
	// Delete services
	for _, svc := range services {
		if err := tc.serviceControl.DeleteService(svc.Namespace, svc.Name, job); err != nil {
			return err
		}
	}
	return nil
}