package v1

const (
	ControllerName            = "TrainingJobOperator"
	TrainingJobReplicaName    = "TrainingJobReplicaName"
	TrainingJobReplicaIndex   = "TrainingJobReplicaIndex"
	TrainingJobNameLabel      = "TrainingJobName"
	TrainingJobFrameworkLabel = "FrameworkType"
	GroupNameLabel            = "GroupName"
	TrainingJobPriorityLabel  = "priority"
)

const (
	TrainingJobReplicaNameEnv  = "TRAININGJOB_REPLICA_NAME"
	TrainingJobReplicaIndexEnv = "TRAININGJOB_REPLICA_INDEX"
	TrainingJobReplicaRestartCountEnv = "TRAININGJOB_REPLICA_RESTARTCOUNT"
	TrainingJobNameEnv         = "TRAININGJOB_NAME"
	TrainingJobNamespaceEnv    = "TRAININGJOB_NAMESPACE"
	TrainingJobServiceEnv      = "TRAININGJOB_SERVICE"
	TrainingJobPortEnv         = "TRAININGJOB_PORTS"
)

const (
	PodTemplateRestartPolicyReason = "SettedPodTemplateRestartPolicy"
	ExitedWithCodeReason           = "ExitedWithCode"
)

const (
	TrainingJobPendingReason     = "TrainingJobPending"
	TrainingJobCreatingReason    = "TrainingJobCreating"
	TrainingJobRunningReason     = "TrainingJobRunning"
	TrainingJobSucceededReason   = "TrainingJobSucceed"
	TrainingJobFailedReason      = "TrainingJobFailed"
	TrainingJobTimeoutReason     = "TrainingJobTimeout"
	TrainingJobRestartingReason  = "TrainingJobRestarting"
	TrainingJobTerminatingReason = "TrainingJobTerminating"
	TrainingJobPreemptedReason   = "TrainingJobPreempted"
	TrainingJobNodeFailReason    = "TrainingJobNodeFail"
)

const (
	DefaultContainerPrefix = "aitj-"
	DefaultPortPrefix      = "aitj-"
)

var (
	ErrorContainerStatus = []string{"CreateContainerConfigError",
		"CreateContainerError",
		//"PreStartHookError",
		//"PostStartHookError",
		"ImagePullBackOff",
		"ImageInspectError",
		"ErrImagePull",
		"ErrImageNeverPull",
		"RegistryUnavailable",
		"InvalidImageName"}

	EndingPhases = []TrainingJobPhase{
		TrainingJobPhaseSucceeded,
		TrainingJobPhaseFailed,
		TrainingJobPhaseTimeout,
		TrainingJobPhasePreempted,
		TrainingJobPhaseNodeFail,
	}
	TrainingJobReason = map[TrainingJobPhase]string{
		TrainingJobPhaseNone:        "",
		TrainingJobPhasePending:     TrainingJobPendingReason,
		TrainingJobPhaseCreating:    TrainingJobCreatingReason,
		TrainingJobPhaseRunning:     TrainingJobRunningReason,
		TrainingJobPhaseSucceeded:   TrainingJobSucceededReason,
		TrainingJobPhaseFailed:      TrainingJobFailedReason,
		TrainingJobPhaseTimeout:     TrainingJobTimeoutReason,
		TrainingJobPhaseRestarting:  TrainingJobRestartingReason,
		TrainingJobPhaseTerminating: TrainingJobTerminatingReason,
		TrainingJobPhasePreempted:   TrainingJobPreemptedReason,
		TrainingJobPhaseNodeFail:    TrainingJobNodeFailReason,
	}
)