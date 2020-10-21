# TrainingJob Operator README
Trainingjob operator is designed for EDL (elastic deep learning) and supports multiple frameworks. It supports automatic fault tolerance and flexible pod completion and termination strategies, and has been tested in paddlepadle, tensorflow, and python frameworks.

## Getting started
### Deploy trainingjob-operator
Trainingjob-operator can be deployed by compiling and executive

	git clone https://github.com/elasticdeeplearning/trainingjob-operator.git
	cd trainingjob-operator/cmd
	go build -o trainingjob-operator
	./trainingjob-operator --master ${master_ip}:${port} --v 4 --thread-num 1000 --logtostderr --leader-elect=true --enable-creating-failed=true
### Run an example trainingjob
#### Submit the trainingjob
	kubectl apply -f https://raw.githubusercontent.com/elasticdeeplearning/trainingjob-operator/master/example/paddle-mnist.yaml
#### Monitor the status of the trainingjob
	kubectl get aitj
	kubectl describe aitj paddle-mnist
#### Delete the trainingjob
	kubectl delete -f https://raw.githubusercontent.com/elasticdeeplearning/trainingjob-operator/master/example/paddle-mnist.yaml

## Trainingjob structure
Using k8s native pod template, we do not implement pserver job and trainer job separately, but directly manage pod through unified replicaset, and reserve the necessary parameters of EDL.
![avatar](https://github.com/elasticdeeplearning/trainingjob-operator/blob/d8c31bfe88c270f12a444a49b6b485312f7a05a7/docs/diagrams/trainingjob.png?raw=true)
+ Trainingjob: Job type.
+ TrainingJobReplicaSet: manage pod, and add additional functions such as preemption, timeout, container creation fault tolerance, node fault tolerance, GPU fault tolerance, etc.; it can adopt completion and end strategies for different frameworks and pod types, so as to support multi framework.
+ Pod: the pod created by the job
+ Service: each pod generates a service with the same name for pod discovery and communication

## Life cycle and state transition diagram
![avatar](https://github.com/elasticdeeplearning/trainingjob-operator/blob/d8c31bfe88c270f12a444a49b6b485312f7a05a7/docs/diagrams/life_cycle_and_state_transition.png?raw=true)
The state transition diagram
+ Fault tolerance of container creation failure: when the container creation of pod fails, within the number of retries, the operator will automatically delete the pod, let the pod reschedule, and expose the container creation information to the user through job information

+ Fault tolerance of pod failure: when pod fails, within the number of retries, the operator will automatically delete the pod and let the pod reschedule, and expose the pod failure and creation information to the user through job information

+ Fault tolerance of node fault and GPU fault: in case of node failure or GPU fault, the operator will automatically delete the pod and let the pod reschedule, and expose the node information to the user through job information

+ Flexible fault tolerant restart strategy: can flexibly set the restart reason: pod failure / node failure / exit code, etc., and flexibly set the restart range: single pod / whole replica / entire job

+ Job failure interface: call the failed or nodefailed annotation added to the job by calling the interface. If the operator detects the annotation, it will delete the job and place it in the failed or nodefailed state

+ Flexible replica failure and successful completion policy: Any/Rank0/All in EndPolicy

+ Flexible job failure and successful completion strategy: Any/All in EndPolicy

+ Flexible cleaning strategy: all/none, automatically cleans all pods of the job after success or failure or not

+ Preemption interface: call the interface to add the preempted annotation to the job. The operator will delete the job and set it to the preempted state when it detects the annotation. After all the pods are deleted, it will be set to the preempted state. (depending on the internal version scheduler, it will not be available for the time being, and it will open source the scheduler later.)
	+ EndPolicy

		Any: the group state changes according to the state of any member in the group;
		Rank0: the group state changes according to the rank0 member state in the group;
		All: the group status changes according to all pods.
	+ RestartScope

		Pod: When restarting, only restart the pod;
		Replica: When restarting, restart all pods of the replica;
		All: When restarting, restart all pods of the job.
	+ CleanPodPolicy

		CleanPodPolicyAll: Clean all pods;
		CleanPodPolicyNone: Do not clean pods.

## Pod discovery strategy
![avatar](https://github.com/elasticdeeplearning/trainingjob-operator/blob/d8c31bfe88c270f12a444a49b6b485312f7a05a7/docs/diagrams/pod_discovery_strategy.png?raw=true)
When the core DNS is deployed, each pod uses the host network. Each pod corresponds to a headless service with the same name. The operator writes all service names into the container environment variable when the pod is generated. When the job runs, it can directly discover the pod through the service of the same name of the pod and communicate through the service;

+ Headless service: the cluster IP of the headless service is none, and the headless service does not perform load balancing and returns IP directly according to a record

## Environment variable
| Variable name | description | example |
| :------| :------| :------|
|TRAININGJOB_REPLICA_Name| replica name, corresponding to ReplicaName |  |
|TRAININGJOB_REPLICA_Type|replica type, corresponding to ReplicaType||
|TRAININGJOB_REPLICA_Index|pod serial number|0|
|${TRAININGJOB_REPLICA_NAME}_Instances|SVC name, comma separated|test-new-tj-pserver-0, test-new-tj-pserver-1|
|${TRAININGJOB_REPLICA_NAME}_INSTANCES_Num|SVC quantity|2|
|${TRAININGJOB_REPLICA_NAME}_Ports|the port number exposed by each SVC, separated by commas|4444,5555|
|${TRAININGJOB_REPLICA_NAME}_ PORTS_ Num|number of ports|2|
|${TRAININGJOB_REPLICA_NAME}_ Hosts|SVC name and port number combination, comma separated|test-new-tj-pserver-0:4444, test-new-tj-pserver-0:44445, test-new-tj-pserver-1:45555|
|${TRAININGJOB_REPLICA_NAME}_HOSTS_Num|host number|4|
|TRAININGJOB_Name|job name||
|TRAININGJOB_NAMESPACE|job namespace||
|TRAININGJOB_Service|the service name used by the current pod for communication||
|TRAININGJOB_Ports|the port number of the current container used for communication, separated by commas||
|TRAININGJOB_REPLICA_RESTARTCOUNT|the times of restart|1|
