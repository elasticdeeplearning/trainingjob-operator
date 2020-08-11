package controller

import (
	"fmt"
	"strings"

	trainingjobv1 "github.com/elasticdeeplearning/trainingjob-operator/pkg/apis/aitrainingjob/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func newTrainingJobCondition(conditionType trainingjobv1.TrainingJobPhase, reason, message string) trainingjobv1.TrainingJobCondition {
	return trainingjobv1.TrainingJobCondition{
		Type:               conditionType,
		Status:             corev1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}
}

func getTrainingJobCondition(trainingJobStatus trainingjobv1.TrainingJobStatus, conditionType trainingjobv1.TrainingJobPhase) *trainingjobv1.TrainingJobCondition {
	for _, condition := range trainingJobStatus.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func isJobCompleted(trainingJobStatus trainingjobv1.TrainingJobStatus) bool {
	// succeed
	succeedCondition := getTrainingJobCondition(trainingJobStatus, trainingjobv1.TrainingJobPhaseSucceeded)
	if succeedCondition != nil && succeedCondition.Status == corev1.ConditionTrue {
		return true
	}
	// failed
	failedCondition := getTrainingJobCondition(trainingJobStatus, trainingjobv1.TrainingJobPhaseFailed)
	if failedCondition != nil && failedCondition.Status == corev1.ConditionTrue {
		return true
	}

	// premepted
	preemptedCondition := getTrainingJobCondition(trainingJobStatus, trainingjobv1.TrainingJobPhasePreempted)
	if preemptedCondition != nil && preemptedCondition.Status == corev1.ConditionTrue {
		return true
	}

	// timeout
	timeoutCondition := getTrainingJobCondition(trainingJobStatus, trainingjobv1.TrainingJobPhaseTimeout)
	if timeoutCondition != nil && timeoutCondition.Status == corev1.ConditionTrue {
		return true
	}

	return false
}

func setTrainingJobCondition(trainingJobStatus *trainingjobv1.TrainingJobStatus, newCondition trainingjobv1.TrainingJobCondition) {
	length := len(trainingJobStatus.Conditions)
	if length > 0 {
		currCond := trainingJobStatus.Conditions[length-1]
		// update current conditions
		if currCond.Type == newCondition.Type &&
			currCond.Status == newCondition.Status &&
			currCond.Reason == newCondition.Reason {
			trainingJobStatus.Conditions[length-1].Message = newCondition.Message
			return
		}
		trainingJobStatus.Conditions[length-1].Status = corev1.ConditionFalse
	}
	// add new condition
	trainingJobStatus.Conditions = append(trainingJobStatus.Conditions, newCondition)
}

func updateTrainingJobConditions(trainingJob *trainingjobv1.AITrainingJob, conditionType trainingjobv1.TrainingJobPhase, reason, message string) error {
	// Do nothing if trainingJobStatus is stopped.
	if isJobCompleted(trainingJob.Status) {
		return nil
	}

	condition := newTrainingJobCondition(conditionType, reason, message)
	setTrainingJobCondition(&trainingJob.Status, condition)
	trainingJob.Status.Phase = conditionType
	return nil
}

func isFailedPhases(phase trainingjobv1.TrainingJobPhase) bool {
	if phase == trainingjobv1.TrainingJobPhaseSucceeded {
		return false
	}
	for _, endingphase := range trainingjobv1.EndingPhases {
		if endingphase == phase {
			return true
		}
	}
	return false
}

func (tc *TrainingJobController) updateStatus(
	trainingJob *trainingjobv1.AITrainingJob,
	pods []*corev1.Pod,
	services []*corev1.Service,
	jobPhases map[trainingjobv1.ReplicaName]trainingjobv1.TrainingJobPhase,
	message string) error {
	for updateRtype := range trainingJob.Spec.ReplicaSpecs {
		initializeTrainingJobReplicaStatuses(trainingJob, updateRtype)
		updateRt := strings.ToLower(string(updateRtype))
		updatePods, _ := tc.FilterPodsForReplicaType(pods, updateRt)
		updateTrainingJobReplicaStatuses(trainingJob, updateRtype, updatePods)
	}
	// When restarting, waiting restarting pods terminated.
	if trainingJob.Status.RestartReplicaName != "" {
		// Waiting all pods terminated
		if trainingJob.Spec.ReplicaSpecs[trainingJob.Status.RestartReplicaName].RestartScope == trainingjobv1.RestartScopeAll && len(pods) == 0 {
			msg := fmt.Sprintf("All pods are restarting now")
			updateTrainingJobConditions(trainingJob, trainingjobv1.TrainingJobPhaseRestarting, trainingjobv1.TrainingJobReason[trainingjobv1.TrainingJobPhaseRestarting], msg)
			trainingJob.Status.RestartReplicaName = ""
			return nil
		}
		rt := strings.ToLower(string(trainingJob.Status.RestartReplicaName))
		replicaPods, err := tc.FilterPodsForReplicaType(pods, rt)
		if err != nil {
			return err
		}
		// Waiting replica pods terminated
		if trainingJob.Spec.ReplicaSpecs[trainingJob.Status.RestartReplicaName].RestartScope == trainingjobv1.RestartScopeReplica && len(replicaPods) == 0 {
			msg := fmt.Sprintf("%v pods are restarting now", rt)
			updateTrainingJobConditions(trainingJob, trainingjobv1.TrainingJobPhaseRestarting, trainingjobv1.TrainingJobReason[trainingjobv1.TrainingJobPhaseRestarting], msg)
			trainingJob.Status.RestartReplicaName = ""
			return nil
		}
		replicas := int(*trainingJob.Spec.ReplicaSpecs[trainingJob.Status.RestartReplicaName].Replicas)
		// Waiting the pod terminated
		if trainingJob.Spec.ReplicaSpecs[trainingJob.Status.RestartReplicaName].RestartScope == trainingjobv1.RestartScopePod && len(replicaPods) < replicas {
			msg := fmt.Sprintf("pod is restarting now")
			updateTrainingJobConditions(trainingJob, trainingjobv1.TrainingJobPhaseRestarting, trainingjobv1.TrainingJobReason[trainingjobv1.TrainingJobPhaseRestarting], msg)
			trainingJob.Status.RestartReplicaName = ""
			return nil
		}
		return nil
	}
	now := metav1.Now()
	spec := trainingJob.Spec
	completedCount := 0
	failedCount := 0
	replicaCount := len(spec.ReplicaSpecs)
	endingPhase := trainingjobv1.TrainingJobPhase("")
	for _, phase := range jobPhases {
		if phase == trainingjobv1.TrainingJobPhaseSucceeded {
			completedCount++
		} else if isFailedPhases(phase) {
			failedCount++
			endingPhase = phase
		}
	}

	//CompletePolicy has a higher priority than FailPolicy
	if spec.CompletePolicy == trainingjobv1.EndingPolicyAny && completedCount > 0 {
		msg := fmt.Sprintf("job %v completed", trainingJob.Name)
		return tc.terminateTrainingJob(trainingJob, pods, services, trainingjobv1.TrainingJobPhaseSucceeded, msg)
	}
	if spec.CompletePolicy == trainingjobv1.EndingPolicyAll && completedCount == replicaCount {
		msg := fmt.Sprintf("job %v completed", trainingJob.Name)
		return tc.terminateTrainingJob(trainingJob, pods, services, trainingjobv1.TrainingJobPhaseSucceeded, msg)
	}
	// FailPolicy
	if spec.FailPolicy == trainingjobv1.EndingPolicyAny  && failedCount > 0 {
		return tc.terminateTrainingJob(trainingJob, pods, services, endingPhase, message)
	}
	if spec.FailPolicy == trainingjobv1.EndingPolicyAll && failedCount == replicaCount {
		return tc.terminateTrainingJob(trainingJob, pods, services, endingPhase, message)
	}

	for _, phase := range trainingjobv1.EndingPhases {
		if msg, ok := trainingJob.Annotations[string(phase)]; ok {
			if len(pods) == 0 {
				trainingJob.Status.EndTime = &now
				msg = fmt.Sprintf("%s; deleted pods", msg)
				return updateTrainingJobConditions(trainingJob, phase, trainingjobv1.TrainingJobReason[phase], msg)
			} else {
				tc.enqueueJob(trainingJob, true, 0)
				return nil
			}
		}
	}

	if trainingJob.Spec.TimeLimit != nil && trainingJob.Status.StartRunningTime != nil {
		if int64((now.Sub(trainingJob.Status.StartRunningTime.Time)).Seconds()) >= *trainingJob.Spec.TimeLimit {
			msg := fmt.Sprintf("started at %s,current time is %s, timeLimit is %v second",
				trainingJob.Status.StartRunningTime.Format("2006-01-02 15:04:05"),
				now.Format("2006-01-02 15:04:05"),
				*trainingJob.Spec.TimeLimit)
			klog.Infof("job %v: %v", trainingJob.Name, msg)
			return tc.terminateTrainingJob(trainingJob, pods, services, trainingjobv1.TrainingJobPhaseTimeout, msg)
		}
	}

	isScheduled := true
	isCreating := false
	isRunning := true
	isRestarting := false

	for rtype := range trainingJob.Spec.ReplicaSpecs {
		replicas := *trainingJob.Spec.ReplicaSpecs[rtype].Replicas
		isScheduled = isScheduled && (trainingJob.Status.ReplicaStatuses[rtype].Scheduled+
			trainingJob.Status.ReplicaStatuses[rtype].Active+
			trainingJob.Status.ReplicaStatuses[rtype].Succeeded+
			trainingJob.Status.ReplicaStatuses[rtype].Failed+
			trainingJob.Status.ReplicaStatuses[rtype].Restarting) == replicas
		isCreating = isCreating || trainingJob.Status.ReplicaStatuses[rtype].Scheduled > 0
		isRestarting = isRestarting || trainingJob.Status.ReplicaStatuses[rtype].Restarting > 0
		isRunning = isRunning && replicas == trainingJob.Status.ReplicaStatuses[rtype].Active
	}
	klog.V(4).Infof("state => %v %v %v %v", isScheduled, isCreating, isRestarting, isRunning)

	if trainingJob.Status.Phase != trainingjobv1.TrainingJobPhaseRunning && isRunning {
		msg := "all pods are running"
		klog.Infof("job %v: %v", trainingJob.Name, msg)
		if trainingJob.Status.StartRunningTime == nil {
			trainingJob.Status.StartRunningTime = &now
		}
		updateTrainingJobConditions(trainingJob, trainingjobv1.TrainingJobPhaseRunning, trainingjobv1.TrainingJobRunningReason, msg)
	}

	if isCreating && isScheduled && trainingJob.Status.Phase != trainingjobv1.TrainingJobPhaseRestarting {
		klog.Infof("job %v:%v", trainingJob.Name, message)
		updateTrainingJobConditions(trainingJob, trainingjobv1.TrainingJobPhaseCreating, trainingjobv1.TrainingJobCreatingReason, message)
	}

	if isRestarting && trainingJob.Status.Phase != trainingjobv1.TrainingJobPhaseRestarting {
		klog.Infof("job %v:%v", trainingJob.Name, message)
		updateTrainingJobConditions(trainingJob, trainingjobv1.TrainingJobPhaseRestarting, trainingjobv1.TrainingJobRestartingReason, message)
	}

	if !isScheduled && !isRestarting && trainingJob.Status.Phase != trainingjobv1.TrainingJobPhaseRestarting {
		msg := "all pods are waiting for scheduling"
		klog.Infof("job %v: %v", trainingJob.Name, msg)
		if trainingJob.Status.StartTime == nil {
			trainingJob.Status.StartTime = &now
		}
		updateTrainingJobConditions(trainingJob, trainingjobv1.TrainingJobPhasePending, trainingjobv1.TrainingJobPendingReason, msg)
	}

	if trainingJob.Spec.TimeLimit != nil && trainingJob.Status.StartRunningTime != nil {
		now := metav1.Now().Unix()
		passed := now - trainingJob.Status.StartRunningTime.Unix()
		total := *trainingJob.Spec.TimeLimit
		klog.Infof("Job with TimeLimit will sync after %d seconds", int64(total-passed))
		tc.enqueueJob(trainingJob, false, int64(total-passed))
	}
	return nil
}

func (tc *TrainingJobController) terminateTrainingJob(
	trainingJob *trainingjobv1.AITrainingJob,
	pods []*corev1.Pod,
	services []*corev1.Service,
	endingPhase trainingjobv1.TrainingJobPhase,
	message string) error {
	// Delete nothing when the cleanPodPolicy is None.
	if (trainingJob.Spec.CleanPodPolicy == nil ||
		*trainingJob.Spec.CleanPodPolicy == trainingjobv1.CleanPodPolicyNone) &&
		(endingPhase == trainingjobv1.TrainingJobPhaseSucceeded ||
			endingPhase == trainingjobv1.TrainingJobPhaseFailed) {
		msg := fmt.Sprintf("%s; kept pods", message)
		klog.Info(msg)
		return updateTrainingJobConditions(trainingJob, endingPhase, trainingjobv1.TrainingJobReason[endingPhase], msg)
	}
	if trainingJob.Annotations == nil {
		trainingJob.Annotations = map[string]string{}
	}
	trainingJob.Annotations[string(endingPhase)] = message
	tc.deletePodsAndServices(trainingJob, pods, services)
	msg := fmt.Sprintf("%s; deleting pods", message)
	klog.Info(msg)
	return updateTrainingJobConditions(
		trainingJob,
		trainingjobv1.TrainingJobPhaseTerminating,
		trainingjobv1.TrainingJobReason[trainingjobv1.TrainingJobPhaseTerminating],
		msg)
}

func (tc *TrainingJobController) updateTrainingJobPhase(trainingJob *trainingjobv1.AITrainingJob) error {
	var err error
	// Update job status, try again 5 times
	for i := 0; i < 5; i++ {
		klog.V(4).Infof("try %v time update job phase %v", i, trainingJob.Status.Phase)
		_, err = tc.trainingJobClient.ElasticdeeplearningV1().AITrainingJobs(trainingJob.Namespace).Update(trainingJob)
		if err == nil {
			return nil
		}
		klog.Errorf("update job phase %v failed, %v", trainingJob.Status.Phase, err)
		newJob, getErr := tc.trainingJobLister.AITrainingJobs(trainingJob.Namespace).Get(trainingJob.Name)
		if getErr != nil || newJob == nil {
			klog.Errorf("get job %v/%v failed, %v", trainingJob.Namespace, trainingJob.Name, getErr)
			continue
		}
		newJob.Status = trainingJob.Status
		newJob.Annotations = trainingJob.Annotations
		trainingJob = newJob
	}
	return err
}

func initializeTrainingJobReplicaStatuses(trainingJob *trainingjobv1.AITrainingJob, rtype trainingjobv1.ReplicaName) {
	if trainingJob.Status.ReplicaStatuses == nil {
		trainingJob.Status.ReplicaStatuses = make(map[trainingjobv1.ReplicaName]*trainingjobv1.ReplicaStatus)
	}

	trainingJob.Status.ReplicaStatuses[rtype] = &trainingjobv1.ReplicaStatus{}
}

func initializeTrainingJobRestartCountes(trainingJob *trainingjobv1.AITrainingJob, rtype trainingjobv1.ReplicaName) {
	if trainingJob.Status.RestartCountes == nil {
		trainingJob.Status.RestartCountes = make(map[trainingjobv1.ReplicaName]int32)
		trainingJob.Status.RestartCountes[rtype] = 0
	}
}

func updateRestartCount(trainingJob *trainingjobv1.AITrainingJob, rtype trainingjobv1.ReplicaName) {
	if trainingJob.Spec.ReplicaSpecs[rtype].RestartScope == trainingjobv1.RestartScopeAll {
		for rtype := range trainingJob.Spec.ReplicaSpecs {
			trainingJob.Status.RestartCountes[rtype]++
		}
	} else {
		trainingJob.Status.RestartCountes[rtype]++
	}
}

func updateTrainingJobReplicaStatuses(trainingJob *trainingjobv1.AITrainingJob, rtype trainingjobv1.ReplicaName, pods []*corev1.Pod,) {
	trainingJob.Status.ReplicaStatuses[rtype].Scheduled = 0
	trainingJob.Status.ReplicaStatuses[rtype].Restarting = 0
	trainingJob.Status.ReplicaStatuses[rtype].Pending = 0
	trainingJob.Status.ReplicaStatuses[rtype].Active = 0
	trainingJob.Status.ReplicaStatuses[rtype].Succeeded = 0
	trainingJob.Status.ReplicaStatuses[rtype].Failed = 0
	for _, pod := range pods {
		switch pod.Status.Phase {
		case corev1.PodPending:
			if trainingJob.Status.RestartCountes[rtype] > 0 {
				trainingJob.Status.ReplicaStatuses[rtype].Restarting++
			} else if pod.Spec.NodeName != "" {
				trainingJob.Status.ReplicaStatuses[rtype].Scheduled++
			} else {
				trainingJob.Status.ReplicaStatuses[rtype].Pending++
			}
		case corev1.PodRunning:
			trainingJob.Status.ReplicaStatuses[rtype].Active++
		case corev1.PodSucceeded:
			trainingJob.Status.ReplicaStatuses[rtype].Succeeded++
		case corev1.PodFailed:
			trainingJob.Status.ReplicaStatuses[rtype].Failed++
		case corev1.PodUnknown:
			trainingJob.Status.ReplicaStatuses[rtype].Failed++
		}
	}
}

func updateTrainingJobPodStatuses(trainingJob *trainingjobv1.AITrainingJob, rtype trainingjobv1.ReplicaName, pod *corev1.Pod) {
	switch pod.Status.Phase {
	case corev1.PodPending:
		if trainingJob.Status.RestartCountes[rtype] > 0 {
			trainingJob.Status.ReplicaStatuses[rtype].Restarting++
		} else if pod.Spec.NodeName != "" {
			trainingJob.Status.ReplicaStatuses[rtype].Scheduled++
		} else {
			trainingJob.Status.ReplicaStatuses[rtype].Pending++
		}
	case corev1.PodRunning:
		trainingJob.Status.ReplicaStatuses[rtype].Active++
	case corev1.PodSucceeded:
		trainingJob.Status.ReplicaStatuses[rtype].Succeeded++
	case corev1.PodFailed:
		trainingJob.Status.ReplicaStatuses[rtype].Failed++
	case corev1.PodUnknown:
		trainingJob.Status.ReplicaStatuses[rtype].Failed++
	}
}
