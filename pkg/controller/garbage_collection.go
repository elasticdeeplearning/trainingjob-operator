package controller

import (
	"time"

	trainingjobv1 "github.com/elasticdeeplearning/trainingjob-operator/pkg/apis/aitrainingjob/v1"
	trainingjoblisters "github.com/elasticdeeplearning/trainingjob-operator/pkg/client/listers/aitrainingjob/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

type GarbageCollector struct {
	kubeCli           kubernetes.Interface
	trainingJobLister trainingjoblisters.AITrainingJobLister
}

func NewGarbageCollector(kubeCli kubernetes.Interface,
	trainingJobLister trainingjoblisters.AITrainingJobLister) *GarbageCollector {
	return &GarbageCollector{
		kubeCli:           kubeCli,
		trainingJobLister: trainingJobLister,
	}
}

func (gc *GarbageCollector) CleanOrphans(d time.Duration) {
	ticker := time.NewTicker(d)
	for range ticker.C {
		klog.V(4).Infof("Garbage collector working now ...")
		gc.CleanGarbagePods()
	}
}

func (gc *GarbageCollector) CleanGarbagePods() {

	all, err := gc.kubeCli.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		klog.Errorf("List garbage pod failed")
		return
	}

	for _, pod := range all.Items {
		roleLabel, isExistRoleLabel := pod.Labels[trainingjobv1.GroupNameLabel]
		if !isExistRoleLabel || roleLabel != trainingjobv1.CRDGroupName {
			continue
		}

		if pod.DeletionTimestamp != nil && pod.DeletionTimestamp.Time.Before(time.Now()) {
			klog.Errorf("Find garbage pod %s, reason: terminated expired", pod.Name)
			gc.deletePod(pod.Namespace, pod.Name)
			continue
		}

		//Find orphan pods
		if ownerRef := metav1.GetControllerOf(pod.GetObjectMeta()); ownerRef != nil {
			// If this object is not owned by Job or ReplicaSet, we should not do anything
			// more with it.
			if ownerRef.Kind == trainingjobv1.SchemeGroupVersion.WithKind(trainingjobv1.CRDKind).Kind &&
				ownerRef.APIVersion == trainingjobv1.SchemeGroupVersion.String() {
				_, err := gc.trainingJobLister.AITrainingJobs(pod.Namespace).Get(ownerRef.Name)
				if apierrors.IsNotFound(err) {
					if pod.DeletionTimestamp != nil && pod.DeletionTimestamp.Time.After(time.Now()) && gc.checkNode(&pod) {
						klog.V(4).Infof("Find pod %s to delete but deletion timestamp is %v, waiting", pod.Name, pod.DeletionTimestamp)
						continue
					}
					klog.Infof("Find pod %s, whose owner Job %s not existed", pod.Name, ownerRef.Name)
					gc.deletePod(pod.Namespace, pod.Name)
				}

			}
		}

	}
}

func (gc *GarbageCollector) deletePod(namespace, name string) {
	var gracePeriodSeconds int64 = 0
	propagationPolicy := metav1.DeletePropagationBackground
	err := gc.kubeCli.CoreV1().Pods(namespace).Delete(name,
		&metav1.DeleteOptions{
			PropagationPolicy:  &propagationPolicy,
			GracePeriodSeconds: &gracePeriodSeconds,
		})
	if err != nil {
		klog.Errorf("Delete pod %s/%s failed, reason: %+v", namespace, name, err)
	}
}

func (gc *GarbageCollector) checkNode(pod *corev1.Pod) bool {
	if pod.Spec.NodeName != "" {
		node, err := gc.kubeCli.CoreV1().Nodes().Get(pod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("check node %s with pod %s failed!!", pod.Spec.NodeName, pod.Name)
			return true
		}
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}
	return true
}
