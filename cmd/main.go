package main

import (
	"github.com/spf13/pflag"
	"github.com/elasticdeeplearning/trainingjob-operator/cmd/app"
	"github.com/elasticdeeplearning/trainingjob-operator/cmd/app/options"
	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/klog"
)

func main() {
	// add cmd flag
	tj := options.NewTrainingJobOperatorOption()
	tj.AddFlags(pflag.CommandLine)

	// parse flags
	flag.InitFlags()

	// run training job operator
	if err := app.Run(tj); err != nil {
		klog.Fatalf("%+v\n", err)
	}
}
