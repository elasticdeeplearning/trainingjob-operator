package options

import (
	"time"

	"github.com/spf13/pflag"
	"k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/apis/config"
	"k8s.io/kubernetes/pkg/client/leaderelectionconfig"
)

type TrainingJobOperatorOption struct {
	MasterUrl            string
	Kubeconfig           string
	RunInCluster         bool
	ThreadNum            int
	CreatingRestartTime  time.Duration
	CreatingDurationTime time.Duration
	EnableCreatingFailed bool
	Namespace            string
	ResyncPeriod         time.Duration
	LeaderElection       config.LeaderElectionConfiguration
}

func NewTrainingJobOperatorOption() *TrainingJobOperatorOption {
	t := TrainingJobOperatorOption{}
	if t.ThreadNum == 0 {
		t.ThreadNum = 1
	}

	if t.Namespace == "" {
		t.Namespace = v1.NamespaceAll
	}

	if t.ResyncPeriod == 0 {
		t.ResyncPeriod = 10 * time.Second
	}

	if t.LeaderElection.LeaseDuration.Duration == 0 {
		t.LeaderElection.LeaseDuration.Duration = 15 * time.Second
	}

	if t.LeaderElection.RenewDeadline.Duration == 0 {
		t.LeaderElection.RenewDeadline.Duration = 5 * time.Second
	}

	if t.LeaderElection.RetryPeriod.Duration == 0 {
		t.LeaderElection.RetryPeriod.Duration = 3 * time.Second
	}

	if t.LeaderElection.ResourceLock == "" {
		t.LeaderElection.ResourceLock = "endpoints"
	}

	if t.CreatingDurationTime == 0 {
		t.CreatingDurationTime = 15 * 60 * time.Second
	}
	return &t
}

func (t *TrainingJobOperatorOption) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&t.MasterUrl, "master", t.MasterUrl, "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	fs.StringVar(&t.Kubeconfig, "kubeconfig", t.Kubeconfig, "Path to a kubeconfig. Only required if out-of-cluster.")
	fs.BoolVar(&t.RunInCluster, "run-in-cluster", t.RunInCluster, "TrainingJob Operator run in k8s cluster or out of cluster.")
	fs.IntVar(&t.ThreadNum, "thread-num", t.ThreadNum, "The num of worker thread")
	fs.StringVar(&t.Namespace, "namespace", t.Namespace, "The namespace to monitor trainingjobs. Defalut all namespaces.")
	fs.DurationVar(&t.ResyncPeriod, "resync-period", t.ResyncPeriod, "Resync interval of the trainingjob operator.")
	fs.DurationVar(&t.CreatingRestartTime, "creating-restart-period", t.CreatingRestartTime, "The period time of retrying to create container")
	fs.DurationVar(&t.CreatingDurationTime, "creating-duration-period", t.CreatingDurationTime, "The period time of creating container")
	fs.BoolVar(&t.EnableCreatingFailed, "enable-creating-failed", t.EnableCreatingFailed, "set job failed if containers have been creating excceed creating-restart-period.")
	leaderelectionconfig.BindFlags(&t.LeaderElection, fs)
}