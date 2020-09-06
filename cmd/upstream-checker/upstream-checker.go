package main

import (
	"github.com/spf13/pflag"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/cmd/upstream-checker/app"
	"github.com/xuchaoi/ingress-nginx-upstream-checker/cmd/upstream-checker/app/option"
	"k8s.io/component-base/cli/flag"
	"k8s.io/klog"
	"os"
)

func main() {
	klog.InitFlags(nil)

	s := option.NewServerRunOptions()
	s.AddFlags(pflag.CommandLine)
	flag.InitFlags()


	if err := app.Run(s); err != nil {
		os.Exit(1)
	}
}
