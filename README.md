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
