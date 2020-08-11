package controller

import (
	"fmt"
	"strconv"
	"strings"

	trainingjobv1 "github.com/elasticdeeplearning/trainingjob-operator/pkg/apis/aitrainingjob/v1"
	"github.com/kubeflow/common/job_controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
)

func getPortsFromJob(job *trainingjobv1.AITrainingJob, rtype trainingjobv1.ReplicaName) []int32 {
	ports := []int32{}
	for _, container := range job.Spec.ReplicaSpecs[rtype].Template.Spec.Containers {
		if strings.HasPrefix(container.Name, trainingjobv1.DefaultContainerPrefix) {
			for _, port := range container.Ports {
				if strings.HasPrefix(port.Name, trainingjobv1.DefaultPortPrefix) {
					ports = append(ports, port.ContainerPort)
				}
			}
		}
	}
	return ports
}

func getPortsFromContainer(container *corev1.Container) []string {
	ports := []string{}
	if strings.HasPrefix(container.Name, trainingjobv1.DefaultContainerPrefix) {
		for _, port := range container.Ports {
			if strings.HasPrefix(port.Name, trainingjobv1.DefaultPortPrefix) {
				ports = append(ports, fmt.Sprintf("%v", port.ContainerPort))
			}
		}
	}
	return ports
}

func hasContainerPort(job *trainingjobv1.AITrainingJob, rtype trainingjobv1.ReplicaName) bool {
	for _, container := range job.Spec.ReplicaSpecs[rtype].Template.Spec.Containers {
		if strings.HasPrefix(container.Name, trainingjobv1.DefaultContainerPrefix) {
			return true
		}
	}
	return false
}

func (tc *TrainingJobController) addService(obj interface{}) {
	service := obj.(*corev1.Service)

	if service.DeletionTimestamp != nil {
		return
	}

	if controllerRef := metav1.GetControllerOf(service); controllerRef != nil {
		tj := tc.resolveControllerRef(service.Namespace, controllerRef)
		if tj == nil {
			return
		}
		tjKey, err := controller.KeyFunc(tj)
		if err != nil {
			klog.Infof("Failed to get the trainingjob key: %v", err)
			return
		}

		replicaType, ok := service.Labels[trainingjobv1.TrainingJobReplicaName]
		if !ok {
			klog.Infof("This pod may not created by %v", trainingjobv1.ControllerName)
			return
		}
		tc.expectations.CreationObserved(job_controller.GenExpectationServicesKey(tjKey, replicaType))
		tc.workQueue.Add(tjKey)
		return
	}
}

func (tc *TrainingJobController) updateService(old, new interface{}) {

}
func (tc *TrainingJobController) deleteService(obj interface{}) {

}

func (tc *TrainingJobController) getServicesByJobAndSelector(job *trainingjobv1.AITrainingJob, selector labels.Selector) ([]*corev1.Service, error) {
	allServices, err := tc.serviceLister.Services(job.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	return tc.claimServices(job, selector, allServices)
}

func (tc *TrainingJobController) claimServices(
	job *trainingjobv1.AITrainingJob,
	selector labels.Selector,
	filteredServices []*corev1.Service) ([]*corev1.Service, error) {
	canAdoptFunc := controller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := tc.trainingJobLister.AITrainingJobs(job.Namespace).Get(job.Name)
		if err != nil {
			return nil, err
		}
		if fresh.UID != job.UID {
			return nil, fmt.Errorf("original %v %v/%v is gone: got uid %v, wanted %v", tc.Kind, job.Namespace, job.Name, fresh.UID, job.UID)
		}
		return fresh, nil
	})
	cm := job_controller.NewServiceControllerRefManager(tc.serviceControl, job, selector, tc.GroupVersionKind, canAdoptFunc)
	return cm.ClaimServices(filteredServices)
}

func (tc *TrainingJobController) reconcileServices(
	job *trainingjobv1.AITrainingJob,
	services []*corev1.Service,
	rtype trainingjobv1.ReplicaName) error {

	ports := getPortsFromJob(job, rtype)

	rt := strings.ToLower(string(rtype))
	spec := job.Spec.ReplicaSpecs[rtype]
	replicas := int(*spec.Replicas)
	// Get all services for the type rt.
	services, err := tc.FilterServicesForReplicaType(services, rt)
	if err != nil {
		return err
	}
	serviceSlices := tc.GetServiceSlices(services, replicas)

	klog.V(4).Infof("job %v type %v ports: %v", job.Name, rtype, ports)
	for index, serviceSlice := range serviceSlices {
		if len(serviceSlice) == 0 && hasContainerPort(job, rtype) {
			klog.Infof("need to create new service: %s-%d", rt, index)
			err = tc.createNewService(job, rtype, strconv.Itoa(index), spec, ports)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (tc *TrainingJobController) createNewService(
	job *trainingjobv1.AITrainingJob,
	rtype trainingjobv1.ReplicaName,
	index string,
	spec *trainingjobv1.ReplicaSpec,
	ports []int32) error {
	jobKey, err := controller.KeyFunc(job)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for job object %#v: %v", job, err))
		return err
	}
	rt := strings.ToLower(string(rtype))
	expectationServicesKey := job_controller.GenExpectationServicesKey(jobKey, rt)
	err = tc.expectations.ExpectCreations(expectationServicesKey, 1)
	if err != nil {
		return err
	}

	labels := tc.GenLabels(job.Name)
	labels[trainingjobv1.TrainingJobReplicaName] = rt
	labels[trainingjobv1.TrainingJobReplicaIndex] = index

	servicePorts := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		servicePorts = append(servicePorts,
			corev1.ServicePort{
				Name: fmt.Sprintf("%s%d", trainingjobv1.DefaultPortPrefix, port),
				Port: port,
			})
	}
	service := &corev1.Service{
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports:     servicePorts,
		},
	}

	service.Name = tc.GenGeneralName(job.Name, rt, index)
	service.Labels = labels
	controllerRef := tc.GenOwnerReference(job)
	err = tc.serviceControl.CreateServicesWithControllerRef(job.Namespace, service, job, controllerRef)
	if err != nil && errors.IsTimeout(err) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (tc *TrainingJobController) FilterServicesForReplicaType(services []*corev1.Service, replicaType string) ([]*corev1.Service, error) {
	var result []*corev1.Service

	replicaSelector := &metav1.LabelSelector{
		MatchLabels: make(map[string]string),
	}

	replicaSelector.MatchLabels[trainingjobv1.TrainingJobReplicaName] = replicaType

	for _, service := range services {
		selector, err := metav1.LabelSelectorAsSelector(replicaSelector)
		if err != nil {
			return nil, err
		}
		if !selector.Matches(labels.Set(service.Labels)) {
			continue
		}
		result = append(result, service)
	}
	return result, nil
}

func (tc *TrainingJobController) GetServiceSlices(services []*corev1.Service, replicas int) [][]*corev1.Service {
	serviceSlices := make([][]*corev1.Service, replicas)
	for _, service := range services {
		if _, ok := service.Labels[trainingjobv1.TrainingJobReplicaIndex]; !ok {
			klog.Warning("The service do not have the index label.")
			continue
		}
		index, err := strconv.Atoi(service.Labels[trainingjobv1.TrainingJobReplicaIndex])
		if err != nil {
			klog.Warningf("Error when strconv.Atoi: %v", err)
			continue
		}
		if index < 0 || index >= replicas {
			klog.Warningf("The label index is not expected: %d", index)
		} else {
			klog.V(4).Infof("index %d, service %v", index, service.Name)
			serviceSlices[index] = append(serviceSlices[index], service)
		}
	}
	return serviceSlices
}
