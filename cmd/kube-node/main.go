package main

/*
   Kube-node is an IPAM CNI-plugin. It reads address ranges (subnets)
   from the K8s node object and delegates the actual address
   assignment to the host-local IPAM CNI-plugin.

   Subnets are taken from spec.podCIDRs in the K8s node object for the
   main K8s network.

   For secondary networks subnets are taken from a specified
   annotation in the K8s node object. The annotation must currently
   contain one subnet, or two subnets for ipv4 and ipv6.
*/

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Nordix/ipam-node-annotation/pkg/log"
	"github.com/Nordix/ipam-node-annotation/cmd/kube-node/app"
)

var (
	version string = "unknown"
)

func main() {
	flagVersion := flag.Bool("version", false, "Print version")
	flag.Parse()
	if *flagVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	// The execution may be blocked by a slow response from the API
	// server, so we set a timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	in := app.ReadCniConfigIn(ctx) // (will exit on failure)

	if in.IPAM.LogFile != "" && in.IPAM.LogFile != "stdout" {
		zlogger, err := log.ZapLogger(in.IPAM.LogFile, in.IPAM.LogLevel)
		if err == nil {
			ctx = log.NewContext(ctx, zlogger)
		}
	}
	app.Main(ctx, in)
}
