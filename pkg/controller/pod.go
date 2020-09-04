package controller

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	trainingjobv1 "github.com/elasticdeeplearning/trainingjob-operator/pkg/apis/aitrainingjob/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
	"github.com/kubeflow/common/job_controller"
)

func (tc *TrainingJobController) addPod(obj interface{}) {
	pod := obj.(*corev1.Pod)

	if pod.DeletionTimestamp != nil {
		return
	}

	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		tj := tc.resolveControllerRef(pod.Namespace, controllerRef)
		if tj == nil {
			return
		}
		tjKey, err := controller.KeyFunc(tj)
		if err != nil {
			klog.Infof("Failed to get the trainingjob key: %v", err)
			return
		}

		replicaType, ok := pod.Labels[trainingjobv1.TrainingJobReplicaName]
		if !ok {
			klog.Infof("This pod may not created by %v", trainingjobv1.ControllerName)
			return
		}

		klog.V(4).Infof("Pod %s created: %#v.", pod.Name, pod)

		tc.expectations.CreationObserved(job_controller.GenExpectationPodsKey(tjKey, replicaType))
		tc.workQueue.Add(tjKey)
		return
	}
}

func (tc *TrainingJobController) updatePod(old, cur interface{}) {
	curPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if curPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}

	curControllerRef := metav1.GetControllerOf(curPod)
	oldControllerRef := metav1.GetControllerOf(oldPod)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		if tj := tc.resolveControllerRef(oldPod.Namespace, oldControllerRef); tj != nil {
			klog.Infof("pod ControllerRef updated: %+v, %+v", curPod, oldPod)
			tc.enqueueJob(tj, false, 0)
		}
	}

	if curControllerRef != nil {
		tj := tc.resolveControllerRef(curPod.Namespace, curControllerRef)
		if tj == nil {
			return
		}
		klog.V(4).Infof("Pod %s updated, objectMeta %+v -> %+v.", curPod.Name, oldPod.ObjectMeta, curPod.ObjectMeta)
		tc.enqueueJob(tj, false, 0)
		return
	}
}

func (tc *TrainingJobController) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a pod %#v", obj))
			return
		}
	}

	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		return
	}

	tj := tc.resolveControllerRef(pod.Namespace, controllerRef)
	if tj == nil {
		return
	}
	tjKey, err := controller.KeyFunc(tj)
	if err != nil {
		return
	}

	replicaType, ok := pod.Labels[trainingjobv1.TrainingJobReplicaName]
	if !ok {
		klog.Infof("This pod may not created by %s", trainingjobv1.ControllerName)
		return
	}

	klog.V(4).Infof("Pod %s/%s deleted through %v, timestamp %+v: %#v.", pod.Namespace, pod.Name, utilruntime.GetCaller(), pod.DeletionTimestamp, pod)
	tc.expectations.DeletionObserved(job_controller.GenExpectationPodsKey(tjKey, replicaType))

	tc.workQueue.Add(tjKey)
}

func (tc *TrainingJobController) getPodsByJobAndSelector(job *trainingjobv1.AITrainingJob, selector labels.Selector) ([]*corev1.Pod, error) {
	allPods, err := tc.podLister.Pods(job.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	return tc.claimPods(job, selector, allPods)
}

func (tc *TrainingJobController) claimPods(
	job *trainingjobv1.AITrainingJob,
	selector labels.Selector,
	filteredPods []*corev1.Pod) ([]*corev1.Pod, error) {
	canAdoptFunc := controller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := tc.trainingJobClient.ElasticdeeplearningV1().AITrainingJobs(job.Namespace).Get(job.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != job.UID {
			return nil, fmt.Errorf("original %v %v/%v is gone: got uid %v, wanted %v", tc.Kind, job.Namespace, job.Name, fresh.UID, job.UID)
		}
		return fresh, nil
	})
	cm := controller.NewPodControllerRefManager(tc.podControl, job, selector, tc.GroupVersionKind, canAdoptFunc)
	return cm.ClaimPods(filteredPods)
}

func (tc *TrainingJobController) reconcilePods(
	job *trainingjobv1.AITrainingJob,
	pods []*corev1.Pod,
	rtype trainingjobv1.ReplicaName) (error, trainingjobv1.TrainingJobPhase, string) {

	if job.Status.Phase == trainingjobv1.TrainingJobPhaseTerminating {
		return nil, trainingjobv1.TrainingJobPhaseTerminating, ""
	}
	if msg, ok := job.Annotations[trainingjobv1.TrainingJobPhasePreempted]; ok {
		return nil, trainingjobv1.TrainingJobPhasePreempted, msg
	}
	if msg, ok := job.Annotations[trainingjobv1.TrainingJobPhaseFailed]; ok {
		return nil, trainingjobv1.TrainingJobPhaseFailed, msg
	}

	// Convert ReplicaType to lower string.
	rt := strings.ToLower(string(rtype))
	// Get all pods for the type rt.
	spec := job.Spec.ReplicaSpecs[rtype]
	replicaPods, err := tc.FilterPodsForReplicaType(pods, rt)
	if err != nil {
		return err, "", ""
	}
	replicas := int32(*spec.Replicas)
	// Init statuses and restart countes
	initializeTrainingJobReplicaStatuses(job, rtype)
	initializeTrainingJobRestartCountes(job, rtype)

	podSlices := tc.GetPodSlices(replicaPods, int(replicas))
	nodeStatus := tc.getNodeStatus()
	message := ""
	failedReason := make([]string, 0, len(podSlices))
	failedPhase := trainingjobv1.TrainingJobPhase("Failed")
	creatingMsg := make(map[string][]string, len(podSlices))
	for index, podSlice := range podSlices {
		if len(podSlice) == 0 {
			klog.Infof("Need to create new pod: %s/%s %s-%d", job.Namespace, job.Name, rt, index)
			err = tc.createNewPod(job, rt, strconv.Itoa(index), strconv.Itoa(int(job.Status.RestartCountes[rtype])), spec)

			if err != nil {
				return err, "", ""
			}
		} else {
			// Check the status of the current pod.
			pod := podSlice[0]
			msg := tc.getPodSchedulingMessage(pod)
			if msg != "" {
				klog.Info(fmt.Sprintf("pod %s is scheduling:%s", pod.Name, msg))
				message = fmt.Sprintf("%s: %s ", rt, msg)
			}
			phase, isRestart, msg := tc.reconcileContainers(job, pod, rtype, nodeStatus)
			klog.Infof("reconcileContainers => %v %v %v", phase, isRestart, msg)
			if msg != "" {
				failedReason = append(failedReason, msg)
			}
			// This part deals with restartingreconcileContainers
			if isRestart {
				// using func forceDeletePod to delete pods when node failed
				deletepod := tc.podControl.DeletePod
				if phase == trainingjobv1.TrainingJobPhaseNodeFail {
					deletepod = tc.forceDeletePod
				}
				// Check restartLimit match restartcount
				if spec.RestartLimit == nil ||
					(spec.RestartLimit != nil && job.Status.RestartCountes[rtype] < *spec.RestartLimit) {
					updateRestartCount(job, rtype)
					msg = fmt.Sprintf("restart times is %d, %s ", job.Status.RestartCountes[rtype], msg)
					// Restart the pod 
					if spec.RestartScope == trainingjobv1.RestartScopePod {
						klog.Warningf("According to restartscope, need to restart the pod: %v.%v", pod.Namespace, pod.Name)
						deletepod(pod.Namespace, pod.Name, job)
						updateTrainingJobReplicaStatuses(job, rtype, replicaPods)
						return nil, trainingjobv1.TrainingJobPhaseRestarting, msg
					}
					// Restart the replica pods 
					if spec.RestartScope == trainingjobv1.RestartScopeReplica {
						klog.Warningf("According to restartscope, need to restart all pods of the replica: %v", rtype)
						for _, podSlice := range podSlices {
							for _, pod := range podSlice {
								deletepod(pod.Namespace, pod.Name, job)
							}
						}
						updateTrainingJobReplicaStatuses(job, rtype, replicaPods)
						return nil, trainingjobv1.TrainingJobPhaseRestarting, msg
					}
					// Restart all pods 
					if spec.RestartScope == trainingjobv1.RestartScopeAll {
						klog.Warningf("According to restartscope, need to restart all pods", )
						for _, pod := range pods {
							deletepod(pod.Namespace, pod.Name, job)
						}
						for updateRtype := range job.Spec.ReplicaSpecs {
							updateRt := strings.ToLower(string(updateRtype))
							updatePods, _ := tc.FilterPodsForReplicaType(pods, updateRt)
							updateTrainingJobReplicaStatuses(job, updateRtype, updatePods)
						}
						return nil, trainingjobv1.TrainingJobPhaseRestarting, msg
					}
				}
			}
			if phase == trainingjobv1.TrainingJobPhaseCreating {
				klog.Info(fmt.Sprintf("pod %s is Creating %s", pod.Name, msg))
				if _, ok := creatingMsg[msg]; !ok {
					creatingMsg[msg] = make([]string, 0, len(podSlices))
				}
				creatingMsg[msg] = append(creatingMsg[msg], pod.Name)
			}

			if phase == trainingjobv1.TrainingJobPhaseSucceeded && pod.Status.Phase == corev1.PodSucceeded &&
				spec.CompletePolicy == trainingjobv1.EndingPolicyAny {
				msg := fmt.Sprintf("pod %s have completed", pod.Name)
				klog.Info(msg)
				return nil, phase, msg
			}

			if (phase == trainingjobv1.TrainingJobPhaseFailed || phase == trainingjobv1.TrainingJobPhaseNodeFail) &&
				spec.FailPolicy == trainingjobv1.EndingPolicyAny {
				msg := fmt.Sprintf("pod %s is failed, %v", pod.Name, msg)
				klog.Info(msg)
				return nil, phase, msg
			}

			if index == 0 {
				if phase == trainingjobv1.TrainingJobPhaseSucceeded && pod.Status.Phase == corev1.PodSucceeded &&
					spec.CompletePolicy == trainingjobv1.EndingPolicyRank0 {
					msg := fmt.Sprintf("rank0 pod %s have completed", pod.Name)
					klog.Info(msg)
					return nil, trainingjobv1.TrainingJobPhaseSucceeded, msg
				}
				if (phase == trainingjobv1.TrainingJobPhaseFailed || phase == trainingjobv1.TrainingJobPhaseNodeFail) &&
					spec.FailPolicy == trainingjobv1.EndingPolicyRank0 {
					msg := fmt.Sprintf("rank0 pod %s is failed, %v", pod.Name, msg)
					klog.Info(msg)
					return nil, phase, msg
				}
			}

			if phase == trainingjobv1.TrainingJobPhaseNodeFail {
				failedPhase = trainingjobv1.TrainingJobPhaseNodeFail
			}
			updateTrainingJobPodStatuses(job, rtype, pod)
		}
	}
	updateTrainingJobReplicaStatuses(job, rtype, replicaPods)
	klog.V(4).Infof("%v status %v", rtype, job.Status.ReplicaStatuses[rtype])

	if spec.CompletePolicy == trainingjobv1.EndingPolicyAll {
		if job.Status.ReplicaStatuses[rtype].Succeeded == replicas {
			msg := fmt.Sprintf("All %s pods have completed", rtype)
			klog.Info(msg)
			return nil, trainingjobv1.TrainingJobPhaseSucceeded, msg
		}
	}

	if spec.FailPolicy == trainingjobv1.EndingPolicyAll {
		if job.Status.ReplicaStatuses[rtype].Failed == replicas {
			if len(failedReason) != 0 {
				message = strings.Join(failedReason, ", ")
			}
			msg := fmt.Sprintf("All %s pods are failed, %s", rtype, message)
			klog.Info(msg)
			return nil, failedPhase, msg
		}
	}

	if len(creatingMsg) != 0 {
		msgs := make([]string, 0, len(creatingMsg))
		for msg, pods := range creatingMsg {
			msgs = append(msgs, fmt.Sprintf("pods %v %s", pods, msg))
		}
		return nil, trainingjobv1.TrainingJobPhaseNone, fmt.Sprintf("%s", strings.Join(msgs, ", "))
	}

	return nil, trainingjobv1.TrainingJobPhaseNone, message
}

func (tc *TrainingJobController) reconcileContainers(
	job *trainingjobv1.AITrainingJob,
	pod *corev1.Pod,
	rtype trainingjobv1.ReplicaName,
	nodeStatus map[string]bool) (trainingjobv1.TrainingJobPhase, bool, string) {
	spec := job.Spec.ReplicaSpecs[rtype]
	exitCode := make([]int32, 0, len(pod.Status.ContainerStatuses))
	failedReason := make([]string, 0, len(pod.Status.ContainerStatuses))
	isRestart := false
	isSucceeded := true
	isCreating := false
	for _, status := range pod.Status.ContainerStatuses {
		state := status.State
		if strings.HasPrefix(status.Name, trainingjobv1.DefaultContainerPrefix) {
			isSucceeded = isSucceeded && state.Terminated != nil
			if state.Terminated != nil {
				code := state.Terminated.ExitCode
				isSucceeded = isSucceeded && code == 0
				exitCode = append(exitCode, code)
				msg := fmt.Sprintf("container %v on node %v exited with reason %v exitcode %v", status.Name, pod.Spec.NodeName, state.Terminated.Reason, code)
				klog.Infof(msg)
				if code != 0 {
					failedReason = append(failedReason, msg)
				}
			}
		}

		if state.Waiting != nil {
			currentTime := time.Now().Unix()
			isCreating = true
			for _, failedStatus := range trainingjobv1.ErrorContainerStatus {
				if state.Waiting.Reason == failedStatus {
					if creating := getTrainingJobCondition(job.Status, trainingjobv1.TrainingJobPhaseCreating); creating != nil &&
						creating.Status == corev1.ConditionTrue {
						if currentTime-creating.LastTransitionTime.Unix() < int64(tc.option.CreatingRestartTime.Seconds()) {
							if currentTime-pod.Status.StartTime.Unix() > int64(tc.option.CreatingDurationTime.Seconds()) {
								klog.Warningf("pod %s create container failed: %s", pod.Name, state.Waiting.Message)
								isRestart = true
							}
						} else if tc.option.EnableCreatingFailed {
							msg := fmt.Sprintf("pod %s create container failed[%v] and has been retrying for %v seconds", pod.Name, state.Waiting.Reason, tc.option.CreatingRestartTime.Seconds())
							klog.Warningf(msg)
							return trainingjobv1.TrainingJobPhaseFailed, isRestart, msg
						}
					}
					failedReason = append(failedReason, state.Waiting.Reason)
					break
				}
			}

		}
	}

	var message string
	var phase trainingjobv1.TrainingJobPhase
	restartingExitCode := string(job.Spec.RestartingExitCode)

	if pod.Status.Phase == corev1.PodFailed {
		// Restartpolicy matches 'ExitCode' and the exitcode is retryable code or restartpolicy matches 'Failure'
		if (spec.RestartPolicy == trainingjobv1.RestartPolicyExitCode && isRetryableExitCode(exitCode, restartingExitCode)) ||
			(spec.RestartPolicy == trainingjobv1.RestartPolicyOnNodeFailWithExitCode && isRetryableExitCode(exitCode, restartingExitCode)) ||
			spec.RestartPolicy == trainingjobv1.RestartPolicyOnFailure ||
			spec.RestartPolicy == trainingjobv1.RestartPolicyAlways {
			isRestart = true
		}

		if len(failedReason) != 0 {
			message = strings.Join(failedReason, "; ")
		} else if pod.Status.Reason != "" {
			message = pod.Status.Reason
			if pod.Status.Message != "" {
				message = fmt.Sprintf("%s, %s", pod.Status.Reason, pod.Status.Message)
			}
		}
		klog.Infof("message %v",message)
		phase = trainingjobv1.TrainingJobPhaseFailed
		return phase, isRestart, message
	}

	if pod.Spec.NodeName != "" {
		if _, ok := nodeStatus[pod.Spec.NodeName]; !ok {
			// Restartpolicy matches 'NodeFailWithExitCode' and the exitcode is retryable code or restartpolicy matches 'NodeFail'
			if spec.RestartPolicy == trainingjobv1.RestartPolicyOnNodeFailWithExitCode ||
				spec.RestartPolicy == trainingjobv1.RestartPolicyOnNodeFail ||
				spec.RestartPolicy == trainingjobv1.RestartPolicyAlways {
				isRestart = true
			}
			phase = trainingjobv1.TrainingJobPhaseNodeFail
			message = fmt.Sprintf("Node %s is failed and offline", pod.Spec.NodeName)
			return phase, isRestart, message
		}
	}

	if isCreating {
		klog.Infof("failedReason %v, isCreating %v", failedReason, isCreating)
		if len(failedReason) != 0 {
			message = strings.Join(failedReason, "; ")
			phase = trainingjobv1.TrainingJobPhaseCreating
			return phase, isRestart, message
		}
		message = "creating containers"
		phase = trainingjobv1.TrainingJobPhaseCreating
		return phase, isRestart, message
	}
	if isSucceeded {
		phase = trainingjobv1.TrainingJobPhaseSucceeded
		return phase, isRestart, message
	}
	return phase, isRestart, message
}

func (tc *TrainingJobController) getNodeStatus() map[string]bool {
	nodesMap := map[string]bool{}
	nodesList, err := tc.kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		klog.Errorf("getNodeStatus failed %v", err)
		return nodesMap
	}
	for _, node := range nodesList.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				nodesMap[node.Name] = true
				break
			}
		}
	}
	return nodesMap
}

func (tc *TrainingJobController) getPodSchedulingMessage(pod *corev1.Pod) string {
	if pod.Status.Phase == corev1.PodPending && pod.Spec.NodeName == "" {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled &&
				condition.Status == corev1.ConditionFalse {
				return condition.Message
			}
		}
	}
	return ""
}

func (tc *TrainingJobController) forceDeletePod(namespace string, podID string, object runtime.Object) error {
	var gracePeriodSeconds int64 = 0
	propagationPolicy := metav1.DeletePropagationBackground
	err := tc.kubeClient.CoreV1().Pods(namespace).Delete(podID,
		&metav1.DeleteOptions{
			PropagationPolicy:  &propagationPolicy,
			GracePeriodSeconds: &gracePeriodSeconds,
		})
	if err != nil {
		klog.Errorf("Delete job pod %s/%s failed, reason: %+v", namespace, podID, err)
	}
	return err
}

func (tc *TrainingJobController) createNewPod(job *trainingjobv1.AITrainingJob, rt, index, restartCount string, spec *trainingjobv1.ReplicaSpec) error {
	jobKey, err := controller.KeyFunc(job)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for job object %#v: %v", job, err))
		return err
	}
	expectationPodsKey := job_controller.GenExpectationPodsKey(jobKey, rt)
	err = tc.expectations.ExpectCreations(expectationPodsKey, 1)
	if err != nil {
		return err
	}

	// Set some info for the worker.
	labels := tc.GenLabels(job.Name)
	labels["JobName"] = job.Name
	labels["PodRole"] = rt
	labels["RestartCount"] = restartCount
	labels[trainingjobv1.TrainingJobReplicaName] = rt
	labels[trainingjobv1.TrainingJobReplicaIndex] = index

	if job.Spec.Priority != "" {
		labels[trainingjobv1.TrainingJobPriorityLabel] = job.Spec.Priority
	}
	podTemplate := spec.Template.DeepCopy()

	// Set name for the template.
	podTemplate.Name = tc.GenGeneralName(job.Name, rt, index)
	podTemplate.GenerateName = tc.GenGeneralName(job.Name, rt, "")
	if podTemplate.Labels == nil {
		podTemplate.Labels = make(map[string]string)
	}

	for key, value := range labels {
		podTemplate.Labels[key] = value
	}
	for key, value := range job.Labels {
		if _, ok := podTemplate.Labels[key]; !ok {
			podTemplate.Labels[key] = value
		}
	}

	if job.Spec.SchedulerName != "" {
		podTemplate.Spec.SchedulerName = job.Spec.SchedulerName
	}

	if err := tc.setEnv(podTemplate, job, spec, rt, index, restartCount); err != nil {
		return err
	}

	if spec.RestartPolicy != "" {
		klog.Infof("There is restart policy in replica spec. set podTemplate.Spec.RestartPolicy never")
		podTemplate.Spec.RestartPolicy = corev1.RestartPolicyNever
	}

	// Create OwnerReference.
	controllerRef := tc.GenOwnerReference(job)
	err = tc.podControl.CreatePodsWithControllerRef(job.Namespace, podTemplate, job, controllerRef)
	if err != nil && errors.IsTimeout(err) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (tc *TrainingJobController) setEnv(
	podTemplateSpec *corev1.PodTemplateSpec,
	job *trainingjobv1.AITrainingJob,
	spec *trainingjobv1.ReplicaSpec,
	rtype, index, restartCount string) error {
	hostsEnv := make([]corev1.EnvVar, 0, len(job.Spec.ReplicaSpecs))
	for rt := range job.Spec.ReplicaSpecs {
		ports := getPortsFromJob(job, rt)
		portsStr := make([]string, 0, len(ports))
		hosts := make([]string, 0, *job.Spec.ReplicaSpecs[rt].Replicas)
		instances := make([]string, 0, *job.Spec.ReplicaSpecs[rt].Replicas)
		for i := int32(0); i < *job.Spec.ReplicaSpecs[rt].Replicas; i++ {
			name := fmt.Sprintf("%v.%v",
				tc.GenGeneralName(job.Name, string(rt), fmt.Sprint(i)),
				job.Namespace)
			instances = append(instances, name)
			for _, port := range ports {
				hosts = append(hosts, fmt.Sprintf("%v:%v", name, port))
			}
		}
		for _, port := range ports {
			portsStr = append(portsStr, fmt.Sprintf("%v", port))
		}
		replicaName := strings.ToUpper(string(rt))
		hostsEnv = append(hostsEnv,
			[]corev1.EnvVar{
				corev1.EnvVar{
					Name:  fmt.Sprintf("%s_INSTANCES", replicaName),
					Value: strings.Join(instances, ","),
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("%s_INSTANCES_NUM", replicaName),
					Value: strconv.Itoa(len(instances)),
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("%s_PORTS", replicaName),
					Value: strings.Join(portsStr, ","),
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("%s_PORTS_NUM", replicaName),
					Value: strconv.Itoa(len(portsStr)),
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("%s_HOSTS", replicaName),
					Value: strings.Join(hosts, ","),
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("%s_HOSTS_NUM", replicaName),
					Value: strconv.Itoa(len(hosts)),
				},
			}...)
	}
	hostsEnv = append(hostsEnv,
		[]corev1.EnvVar{
			corev1.EnvVar{
				Name:  trainingjobv1.TrainingJobReplicaNameEnv,
				Value: rtype,
			},
			corev1.EnvVar{
				Name:  trainingjobv1.TrainingJobReplicaIndexEnv,
				Value: index,
			},
			corev1.EnvVar{
				Name:  trainingjobv1.TrainingJobReplicaRestartCountEnv,
				Value: restartCount,
			},
			corev1.EnvVar{
				Name: trainingjobv1.TrainingJobServiceEnv,
				Value: fmt.Sprintf("%v.%v",
					tc.GenGeneralName(job.Name, rtype, index),
					job.Namespace),
			},
			corev1.EnvVar{
				Name:  trainingjobv1.TrainingJobNameEnv,
				Value: job.Name,
			},
			corev1.EnvVar{
				Name:  trainingjobv1.TrainingJobNamespaceEnv,
				Value: job.Namespace,
			},
		}...)

	for i := range podTemplateSpec.Spec.InitContainers {
		if len(podTemplateSpec.Spec.InitContainers[i].Env) == 0 {
			podTemplateSpec.Spec.InitContainers[i].Env = make([]corev1.EnvVar, 0)
		}
		podTemplateSpec.Spec.InitContainers[i].Env = append(podTemplateSpec.Spec.InitContainers[i].Env,
			hostsEnv...)
	}

	for i := range podTemplateSpec.Spec.Containers {
		if len(podTemplateSpec.Spec.Containers[i].Env) == 0 {
			podTemplateSpec.Spec.Containers[i].Env = make([]corev1.EnvVar, 0)
		}
		podTemplateSpec.Spec.Containers[i].Env = append(podTemplateSpec.Spec.Containers[i].Env,
			hostsEnv...)
		podTemplateSpec.Spec.Containers[i].Env = append(podTemplateSpec.Spec.Containers[i].Env,
			corev1.EnvVar{
				Name:  trainingjobv1.TrainingJobPortEnv,
				Value: strings.Join(getPortsFromContainer(&podTemplateSpec.Spec.Containers[i]), ","),
			})
	}

	return nil
}

func (tc *TrainingJobController) FilterPodsForReplicaType(pods []*corev1.Pod, replicaType string) ([]*corev1.Pod, error) {
	var result []*corev1.Pod

	replicaSelector := &metav1.LabelSelector{
		MatchLabels: make(map[string]string),
	}

	replicaSelector.MatchLabels[trainingjobv1.TrainingJobReplicaName] = replicaType

	for _, pod := range pods {
		selector, err := metav1.LabelSelectorAsSelector(replicaSelector)
		if err != nil {
			return nil, err
		}
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		result = append(result, pod)
	}
	return result, nil
}

func (tc *TrainingJobController) GetPodSlices(pods []*corev1.Pod, replicas int) [][]*corev1.Pod {
	podSlices := make([][]*corev1.Pod, replicas)
	for _, pod := range pods {
		if _, ok := pod.Labels[trainingjobv1.TrainingJobReplicaIndex]; !ok {
			klog.Warning("The pod do not have the index label.")
			continue
		}
		index, err := strconv.Atoi(pod.Labels[trainingjobv1.TrainingJobReplicaIndex])
		if err != nil {
			klog.Warningf("Error when strconv.Atoi: %v", err)
			continue
		}
		if index < 0 || index >= replicas {
			klog.Warningf("The label index is not expected: %d", index)
		} else {
			klog.V(4).Infof("index %d, pod %v", index, pod.Name)
			podSlices[index] = append(podSlices[index], pod)
		}
	}
	return podSlices
}
