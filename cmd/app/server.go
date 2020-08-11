package app

import (
	"context"
	"fmt"
	"github.com/elasticdeeplearning/trainingjob-operator/cmd/app/options"
	trainingjobclientset "github.com/elasticdeeplearning/trainingjob-operator/pkg/client/clientset/versioned"
	trainingjobinformers "github.com/elasticdeeplearning/trainingjob-operator/pkg/client/informers/externalversions"
	"github.com/elasticdeeplearning/trainingjob-operator/pkg/controller"
	"github.com/elasticdeeplearning/trainingjob-operator/pkg/signals"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/util/uuid"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclientset "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"os"
)

func Run(opt *options.TrainingJobOperatorOption) error {
	if opt.Namespace == corev1.NamespaceAll {
		klog.Info("List-watch resources across all namespaces")
	} else {
		klog.Info("List-watch resources to namespace %s", opt.Namespace)
	}

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	// create client sets
	kubeClient, leaderElectionClient, trainingJobClient, extapiClient, err := createClientSets(opt)
	if err != nil {
		return fmt.Errorf("unable to create kube client: %v", err)
	}

	// create informer factory
	kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(kubeClient, opt.ResyncPeriod, kubeinformers.WithNamespace(opt.Namespace))
	trainingJobInformerFactory := trainingjobinformers.NewSharedInformerFactoryWithOptions(trainingJobClient, opt.ResyncPeriod, trainingjobinformers.WithNamespace(opt.Namespace))

	// create trainingjob controller
	tc := controller.NewTrainingJobController(kubeClient, trainingJobClient, extapiClient, kubeInformerFactory, trainingJobInformerFactory, *opt)

	// start informer
	go kubeInformerFactory.Start(stopCh)
	go trainingJobInformerFactory.Start(stopCh)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// start controller
	run := func(ctxRun context.Context) {
		klog.V(4).Infof("I won the leader election")
		if err := tc.Run(opt.ThreadNum, stopCh); err != nil {
			klog.Errorf("Failed to run the controller: %v", err)
			cancel()
		}
		<-ctxRun.Done()
	}

	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	if !opt.LeaderElection.LeaderElect {
		run(ctx)
		return fmt.Errorf("finished without leader elect")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("unable to get hostname: %v", err)
	}

	recorder := createRecorder()
	rl, err := resourcelock.New(opt.LeaderElection.ResourceLock,
		"kube-system",
		"trainingjob-operator",
		leaderElectionClient.CoreV1(),
		resourcelock.ResourceLockConfig{
			Identity:      hostname + "_" + string(uuid.NewUUID()),
			EventRecorder: recorder,
		})

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: opt.LeaderElection.LeaseDuration.Duration,
		RenewDeadline: opt.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:   opt.LeaderElection.RetryPeriod.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				klog.Fatalf("leaderelection lost")
			},
		},
		Name: "trainingjob-operator",
	})

	return fmt.Errorf("lost lease")
}

func createClientSets(opt *options.TrainingJobOperatorOption) (kubeclientset.Interface, kubeclientset.Interface, trainingjobclientset.Interface, apiextensionsclient.Interface, error) {
	var kubeConfig *restclientset.Config = nil
	var err error

	if opt.RunInCluster {
		kubeConfig, err = restclientset.InClusterConfig()
	} else {
		kubeConfig, err = clientcmd.BuildConfigFromFlags(opt.MasterUrl, opt.Kubeconfig)
	}

	if err != nil {
		klog.Fatalf("Error building kubeconfig: %+v", err)
		return nil, nil, nil, nil, err
	}

	kubeClientSet, err := kubeclientset.NewForConfig(restclientset.AddUserAgent(kubeConfig, "trainingjob-operator"))
	if err != nil {
		klog.Fatalf("Error building kubeClientSet")
		return nil, nil, nil, nil, err
	}

	leaderElectionClientSet, err := kubeclientset.NewForConfig(restclientset.AddUserAgent(kubeConfig, "leader-election"))
	if err != nil {
		klog.Fatalf("Error building leaderElectionClientSet")
		return nil, nil, nil, nil, err
	}

	tjClientSet, err := trainingjobclientset.NewForConfig(kubeConfig)
	if err != nil {
		klog.Fatalf("Error building trainingJobClientSet")
		return nil, nil, nil, nil, err
	}

	extapiClient, err := apiextensionsclient.NewForConfig(kubeConfig)
	if err != nil {
		klog.Fatalf("Error building apiExtensionsClientSet")
		return nil, nil, nil, nil, err
	}

	return kubeClientSet, leaderElectionClientSet, tjClientSet, extapiClient, nil
}

func createRecorder() record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "trainingjob-operator"})
}
