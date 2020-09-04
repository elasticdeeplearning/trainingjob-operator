package controller

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/elasticdeeplearning/trainingjob-operator/cmd/app/options"
	trainingjobv1 "github.com/elasticdeeplearning/trainingjob-operator/pkg/apis/aitrainingjob/v1"
	trainingjobclientset "github.com/elasticdeeplearning/trainingjob-operator/pkg/client/clientset/versioned"
	trainingjobscheme "github.com/elasticdeeplearning/trainingjob-operator/pkg/client/clientset/versioned/scheme"
	trainingjobinformers "github.com/elasticdeeplearning/trainingjob-operator/pkg/client/informers/externalversions"
	trainingjoblisters "github.com/elasticdeeplearning/trainingjob-operator/pkg/client/listers/aitrainingjob/v1"
	"github.com/kubeflow/common/job_controller"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
)

type TrainingJobController struct {
	// GroupVersionKind indicates the controller type.
	schema.GroupVersionKind
	// KubeClient is a standard kubernetes clientset
	kubeClient kubeclientset.Interface
	// ApiExtensionsClient is the extension kubernetes clientset
	apiExtensionsClient apiextensionsclient.Interface
	// TrainingJobClient is a clientset for our own API group
	trainingJobClient trainingjobclientset.Interface
	// Lister for TrainingJob
	trainingJobLister trainingjoblisters.AITrainingJobLister
	// trainingJobInformerSynced returns true if the trainingjob store has been synced at least once
	trainingJobInformerSynced cache.InformerSynced
	// PodControlInterface is an interface that knows how to add or delete pods
	podControl controller.PodControlInterface
	// Lister for Pod
	podLister corelisters.PodLister
	// podListerSynced returns true if the pod store has been synced at least once
	podInformerSynced cache.InformerSynced
	// service control
	serviceControl job_controller.ServiceControlInterface
	// Lister for Service
	serviceLister corelisters.ServiceLister
	// serviceInformerSynced return true if the service store has been synced at least once
	serviceInformerSynced cache.InformerSynced
	// A TTLCache of pod creates/deletes each rc expects to see.
	expectations controller.ControllerExpectationsInterface
	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens.
	workQueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
	option options.TrainingJobOperatorOption
}

func NewTrainingJobController(
	kubeClient kubeclientset.Interface,
	trainingJobClient trainingjobclientset.Interface,
	extApiClient apiextensionsclient.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	trainingJobInformerFactory trainingjobinformers.SharedInformerFactory,
	option options.TrainingJobOperatorOption,
) *TrainingJobController {
	// add scheme
	trainingjobscheme.AddToScheme(scheme.Scheme)
	// create informer
	trainingJobInformer := trainingJobInformerFactory.Elasticdeeplearning().V1().AITrainingJobs()
	podInformer := kubeInformerFactory.Core().V1().Pods()
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	// create recorder
	klog.Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: trainingjobv1.ControllerName})

	podControl := job_controller.RealPodControl{
		KubeClient: kubeClient,
		Recorder:   eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: trainingjobv1.ControllerName}),
	}

	serviceControl := job_controller.RealServiceControl{
		KubeClient: kubeClient,
		Recorder:   eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: trainingjobv1.ControllerName}),
	}

	// create tc struct
	tc := &TrainingJobController{
		GroupVersionKind:    trainingjobv1.SchemeGroupVersion.WithKind(trainingjobv1.CRDKind),
		kubeClient:          kubeClient,
		apiExtensionsClient: extApiClient,
		trainingJobClient:   trainingJobClient,
		podControl:          podControl,
		serviceControl:      serviceControl,
		expectations:        controller.NewControllerExpectations(),
		workQueue:           workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), trainingjobv1.CRDKind),
		recorder:            recorder,
		option:              option,
	}

	trainingJobInformer.Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *trainingjobv1.AITrainingJob:
					klog.V(4).Infof("Filter training job: %s/%s", t.Namespace, t.Name)
					return true
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    tc.addTrainingJob,
				UpdateFunc: tc.updateTrainingJob,
				DeleteFunc: tc.deleteTrainingJob,
			},
		},
	)

	tc.trainingJobLister = trainingJobInformer.Lister()
	tc.trainingJobInformerSynced = trainingJobInformer.Informer().HasSynced

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    tc.addPod,
		UpdateFunc: tc.updatePod,
		DeleteFunc: tc.deletePod,
	})

	tc.podLister = podInformer.Lister()
	tc.podInformerSynced = podInformer.Informer().HasSynced

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    tc.addService,
		UpdateFunc: tc.updateService,
		DeleteFunc: tc.deleteService,
	})

	tc.serviceLister = serviceInformer.Lister()
	tc.serviceInformerSynced = serviceInformer.Informer().HasSynced

	return tc
}

func (tc *TrainingJobController) GenOwnerReference(obj metav1.Object) *metav1.OwnerReference {
	boolPtr := func(b bool) *bool { return &b }
	controllerRef := &metav1.OwnerReference{
		APIVersion:         trainingjobv1.SchemeGroupVersion.String(),
		Kind:               trainingjobv1.SchemeGroupVersion.WithKind(trainingjobv1.CRDKind).Kind,
		Name:               obj.GetName(),
		UID:                obj.GetUID(),
		BlockOwnerDeletion: boolPtr(true),
		Controller:         boolPtr(true),
	}

	return controllerRef
}

func (tc *TrainingJobController) GenLabels(jobName string) map[string]string {
	return map[string]string{
		trainingjobv1.GroupNameLabel:       trainingjobv1.CRDGroupName,
		trainingjobv1.TrainingJobNameLabel: strings.Replace(jobName, "/", "-", -1),
	}
}

func (tc *TrainingJobController) Run(workers int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer tc.workQueue.ShutDown()

	klog.Info("Starting training-job controller")
	defer klog.Info("Shutting down training-job controller")

	klog.Info("Starting to create TrainingJob CRD")
	if err := tc.createCRD(); err != nil {
		return fmt.Errorf("Failed to create kind TrainingJob: %v", err)
	}

	klog.Info("Waiting for informer caches to sync")
	if !controller.WaitForCacheSync(trainingjobv1.CRDKind, stopCh, tc.trainingJobInformerSynced, tc.podInformerSynced, tc.serviceInformerSynced) {
		return fmt.Errorf("failed to wait for caches for sync")
	}

	klog.Info("Starting workers")
	for i := 0; i < workers; i++ {
		go wait.Until(tc.worker, time.Second, stopCh)
	}
	gc := NewGarbageCollector(tc.kubeClient, tc.trainingJobLister)
	go gc.CleanOrphans(10 * time.Minute)

	<-stopCh
	return nil
}

func (tc *TrainingJobController) createCRD() error {
	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: trainingjobv1.CRDName(),
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   trainingjobv1.CRDGroupName,
			Version: trainingjobv1.CRDGroupVersion,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Kind:       trainingjobv1.CRDKind,
				Plural:     trainingjobv1.CRDKindPlural,
				ShortNames: []string{trainingjobv1.CRDShortName},
			},
		},
	}

	_, err := tc.apiExtensionsClient.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		klog.Errorf("Failed to create crd, error: %s", err.Error())
		return err
	}

	return nil
}

func (tc *TrainingJobController) worker() {
	for tc.processNextWorkItem() {
	}
}

func (tc *TrainingJobController) processNextWorkItem() bool {
	klog.Infof("Current queue length %d", tc.workQueue.Len())
	obj, shutdown := tc.workQueue.Get()
	if shutdown {
		return false
	}

	defer tc.workQueue.Done(obj)

	key, ok := obj.(string)
	if !ok {
		tc.workQueue.Forget(obj)
		utilruntime.HandleError(fmt.Errorf("expected string in workqueue, but got %+v", obj))
		return true
	}

	forget, err := tc.syncHandler(key)
	if err == nil {
		if forget {
			tc.workQueue.Forget(key)
		}
		return true
	}

	utilruntime.HandleError(fmt.Errorf("Sync %q failed with %v", key, err))
	tc.workQueue.AddRateLimited(key)
	return true
}

func (tc *TrainingJobController) syncHandler(key string) (bool, error) {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing %v %q (%v)", tc.Kind, key, time.Since(startTime))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return false, err
	}

	if len(namespace) == 0 || len(name) == 0 {
		return false, fmt.Errorf("invalid trainingjob key %q", key)
	}

	trainingJob, err := tc.trainingJobLister.AITrainingJobs(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		klog.V(4).Infof("%v %v has been deleted", tc.Kind, key)
		// FIXME: ?
		return true, nil
	}

	if err != nil {
		return false, err
	}
	tjNeedSync := tc.satisfiedExpectations(trainingJob)
	// Set default for the new trainingJob.
	scheme.Scheme.Default(trainingJob)
	if tjNeedSync && trainingJob.DeletionTimestamp == nil &&
		(trainingJob.Status.Phase == trainingjobv1.TrainingJobPhaseNone ||
			trainingJob.Status.Phase == trainingjobv1.TrainingJobPhasePending ||
			trainingJob.Status.Phase == trainingjobv1.TrainingJobPhaseCreating ||
			trainingJob.Status.Phase == trainingjobv1.TrainingJobPhaseRunning ||
			trainingJob.Status.Phase == trainingjobv1.TrainingJobPhaseRestarting ||
			trainingJob.Status.Phase == trainingjobv1.TrainingJobPhaseTerminating) {
		err := tc.reconcileTrainingJobs(trainingJob)
		if err != nil {
			return false, err
		}
	}

	return true, err
}

func (tc *TrainingJobController) reconcileTrainingJobs(trainingJob *trainingjobv1.AITrainingJob) error {
	klog.V(4).Infof("Reconcile training job: %s/%s", trainingJob.Namespace, trainingJob.Name)
	oldJobStatus := trainingJob.Status.DeepCopy()
	// gen selector
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			trainingjobv1.GroupNameLabel:       tc.Group,
			trainingjobv1.TrainingJobNameLabel: trainingJob.Name,
		},
	})

	if err != nil {
		return fmt.Errorf("error converting Job selector: %v", err)
	}
	// get pods
	pods, err := tc.getPodsByJobAndSelector(trainingJob, selector)
	if err != nil {
		klog.Errorf("getPodsByJobAndSelector error %v", err)
		return err
	}
	// get services
	services, err := tc.getServicesByJobAndSelector(trainingJob, selector)
	if err != nil {
		klog.Errorf("getServicesByJobAndSelector error %v", err)
		return err
	}
	endingPhases := make(map[trainingjobv1.ReplicaName]trainingjobv1.TrainingJobPhase)
	endingPhase := trainingjobv1.TrainingJobPhase("")
	aggregationMsg := make([]string, 0, len(trainingJob.Spec.ReplicaSpecs))
	// When RestartReplicaName is not None, waiting restarting pods terminated
	if trainingJob.Status.RestartReplicaName == "" {
		for rtype := range trainingJob.Spec.ReplicaSpecs {
			msg := ""
			err, endingPhase, msg = tc.reconcilePods(trainingJob, pods, rtype)
			if err != nil {
				klog.Warningf("reconcilePods error %v", err)
				return err
			}
			if msg != "" {
				isOld := false
				for _, oldmsg := range aggregationMsg {
					isOld = isOld || oldmsg == msg
				}
				if !isOld {
					aggregationMsg = append(aggregationMsg, msg)
				}
			}
			// When endingPhase is restarting, some pods need to be terminated, and phase trans to terminating. 
			if endingPhase == trainingjobv1.TrainingJobPhaseRestarting {
				updateTrainingJobConditions(trainingJob, trainingjobv1.TrainingJobPhaseTerminating, trainingjobv1.TrainingJobReason[trainingjobv1.TrainingJobPhaseTerminating], msg)
				trainingJob.Status.RestartReplicaName = rtype
				break
			}
			if endingPhase != "" {
				endingPhases[rtype] = endingPhase
				continue
			}
			err = tc.reconcileServices(trainingJob, services, rtype)
			if err != nil {
				klog.Warningf("reconcileServices error %v", err)
				return err
			}
		}
	}
	message := ""
	if len(aggregationMsg) != 0 {
		message = strings.Join(aggregationMsg, "; ")
	}
	tc.updateStatus(trainingJob, pods, services, endingPhases, message)
	// When status changed, update it.
	if !reflect.DeepEqual(*oldJobStatus, trainingJob.Status) {
		return tc.updateTrainingJobPhase(trainingJob)
	}
	return nil
}

func (tc *TrainingJobController) satisfiedExpectations(trainingJob *trainingjobv1.AITrainingJob) bool {
	satisfied := false
	tjKey, err := controller.KeyFunc(trainingJob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for training job object %#v: %v", trainingJob, err))
		return false
	}

	for replicaType := range trainingJob.Spec.ReplicaSpecs {
		satisfied = satisfied || tc.expectations.SatisfiedExpectations(job_controller.GenExpectationPodsKey(tjKey, string(replicaType)))
		satisfied = satisfied || tc.expectations.SatisfiedExpectations(job_controller.GenExpectationServicesKey(tjKey, string(replicaType)))
	}

	return satisfied
}

func (tc *TrainingJobController) enqueueJob(trainingJob interface{}, isLimited bool, delay int64) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(trainingJob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("could not get key for trainingjob object %#v: %v", trainingJob, err))
		return
	}

	klog.Infof("Enqueue key: %v", key)
	if isLimited {
		tc.workQueue.AddRateLimited(key)
	} else if delay > 0 {
		tc.workQueue.AddAfter(key, time.Duration(delay)*time.Second)
	} else {
		tc.workQueue.Add(key)
	}
}

// resolveControllerRef get the controller referenced by a ControllerRef,
func (tc *TrainingJobController) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *trainingjobv1.AITrainingJob {
	// the Kind must match.
	if controllerRef.Kind != tc.Kind {
		return nil
	}

	tj, err := tc.trainingJobLister.AITrainingJobs(namespace).Get(controllerRef.Name)
	if err != nil {
		return nil
	}

	if tj.UID != controllerRef.UID {
		// if UIDs are not same, controllers are not same too.
		return nil
	}
	return tj
}

func isRetryableExitCode(exitCode []int32, restartingExitCode string) bool {
	if len(exitCode) == 0 {
		return false
	}
	restartingExitCode_slice := strings.Split(restartingExitCode, ",")
	retry := true
	for _, code := range exitCode {
		retry = retry && checkExitCode(code, restartingExitCode_slice)
	}
	return retry
}

func checkExitCode(code int32, restartingExitCode_slice []string) bool {
	exitcode := strconv.FormatInt(int64(code), 10)
	for _, restartExitCode := range restartingExitCode_slice {
		if exitcode == restartExitCode {
			return true
		}
	}
	return false
}
