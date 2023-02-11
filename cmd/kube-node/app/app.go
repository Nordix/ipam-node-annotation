package app

/*
   app implements the kube-node IPAM CNI-plugin

   A cache named "kube-node.json" is stored in DataDir. It is a valid
   host-local config and can be used as-is unless "ipv4-namespaces" is
   specified.
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nordix/ipam-node-annotation/pkg/util"
	"github.com/containernetworking/cni/pkg/invoke"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/go-logr/logr"
	k8s "k8s.io/api/core/v1"
	//"cmd/go/internal/lockedfile/internal/filelock"
)

// Define the "ipam" formats for kube-node and host-local
type kubeNodeIPAM struct {
	Type       string   `json:"type"`
	Annotation string   `json:"annotation,omitempty"`
	DataDir    string   `json:"dataDir,omitempty"`
	IPv4NS     []string `json:"ipv4-namespaces,omitempty"`
	KubeConfig string   `json:"kubeconfig,omitempty"`
	LogFile    string   `json:"logfile,omitempty"`
	LogLevel   string   `json:"loglevel,omitempty"`
}
type hostLocalIPAM struct {
	Type    string   `json:"type"`
	DataDir string   `json:"dataDir,omitempty"`
	Ranges  []ranges `json:"ranges"`
}
type ranges []rangeItem
type rangeItem struct {
	Subnet string `json:"subnet"`
}

// Define input and output (json) to this plugin
type CniConfigIn struct {
	Name             string        `json:"name"`
	CNIVersion       string        `json:"cniVersion"`
	IsDefaultGateway bool          `json:"isDefaultGateway"`
	IPAM             *kubeNodeIPAM `json:"ipam"`
}
type cniConfigOut struct {
	Name             string         `json:"name"`
	CNIVersion       string         `json:"cniVersion"`
	IsDefaultGateway bool           `json:"isDefaultGateway,omitempty"`
	IPAM             *hostLocalIPAM `json:"ipam"`
}

// ReadCniConfigIn Reads stdin and creates a CNI config structure.
// On failure CniErrorExit is called. No logger is available at this time.
func ReadCniConfigIn(ctx context.Context) *CniConfigIn {
	var in CniConfigIn
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&in); err != nil {
		util.CniErrorExit(
			ctx, err, cnitypes.ErrDecodingFailure, "Decode stdin")
	}
	if in.CNIVersion != "" {
		util.CniVersion = in.CNIVersion
	}
	return &in
}

// Main Executes the CNI command
func Main(ctx context.Context, in *CniConfigIn) {
	logger := logr.FromContextOrDiscard(ctx)
	trace := logger.V(2)
	if trace.Enabled() {
		trace.Info(
			"Started", "CNI_COMMAND", os.Getenv("CNI_COMMAND"),
			"CNI_ARGS", os.Getenv("CNI_ARGS"),
			"CniConfigIn", in)
	}
	if in.IPAM == nil {
		// This is acceptable for CNI_COMMAND=VERSION
		if os.Getenv("CNI_COMMAND") == "VERSION" {
			_ = execChained(ctx, nil)
			return
		} else {
			err := fmt.Errorf("No IPAM found")
			util.CniErrorExit(
				ctx, err, cnitypes.ErrDecodingFailure, "Decode stdin")
		}
	}
	if in.IPAM.KubeConfig != "" {
		os.Setenv("KUBECONFIG", in.IPAM.KubeConfig)
	}

	o := newOutIpam(ctx, in)
	if err := o.readCache(ctx); err != nil {
		// Failed to read from cache. We must read the subnets from
		// the own K8s node object. This is not a fatal error but can
		// flood the logs, so use debug loglevel
		logger.V(1).Error(err, "Read Cache", "file", o.cache)
		n, err := getOwnNode(ctx, util.RealNodeReader())
		if err != nil {
			util.CniErrorExit(ctx, err, 100, "Get the own node object")
		}
		cidrs, err := getPodCIDRs(ctx, n, o.inCfg.IPAM.Annotation)
		if err != nil {
			util.CniErrorExit(ctx, err, 100, "Get PodCIDRs")
		}
		err = o.createHostLocalIPAM(ctx, cidrs)
		if err != nil {
			util.CniErrorExit(ctx, err, 100, "CIDR config")
		}
		o.writeCache(ctx)
	}

	err := execChained(ctx, o.computeOutData(ctx))
	if err != nil {
		o.deleteCache()
		util.CniErrorExit(ctx, err, 100, "Invoke host-local ipam")
	}
}

// outIpam handles the chained IPAM CNI-plugin, currently "host-local" only
type outIpam struct {
	logger logr.Logger
	trace  logr.Logger
	inCfg  *CniConfigIn
	cache  string
	ipam   *hostLocalIPAM // To/from cache
}

// newOutIpam Create a out-ipam handler
func newOutIpam(
	ctx context.Context, inCfg *CniConfigIn) *outIpam {
	o := outIpam{
		inCfg:  inCfg,
		logger: logr.FromContextOrDiscard(ctx),
		ipam:   &hostLocalIPAM{},
	}
	o.trace = o.logger.V(2)
	dataDir := inCfg.IPAM.DataDir
	if dataDir == "" {
		dataDir = "/var/lib/cni/networks/" + inCfg.Name
	}
	o.cache = dataDir + "/kube-node.json"
	return &o
}

// readCache Tries to read the configuration from cache
func (o *outIpam) readCache(ctx context.Context) error {
	cacheData, err := os.ReadFile(o.cache)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(cacheData, o.ipam); err != nil {
		return err
	}
	if err := validateHostLocalIPAM(o.ipam); err != nil {
		return err
	}
	o.trace.Info("Cache read", "data", o.ipam)
	return nil
}

// computeOutData Compute data for the chained ipam (host-local).
// Prerequisite: The host-local config must be read from cache or
// created in o.ipam
func (o *outIpam) computeOutData(ctx context.Context) *cniConfigOut {
	if o.trace.Enabled() {
		o.trace.Info(
			"Compute data for the chained ipam",
			"NS", getK8sNamespace(ctx), "ipv4-namespaces", o.inCfg.IPAM.IPv4NS)
	}

	// Check if we shall assign an IPv4 address. If any problem occur
	// the fallback is to assign IPv4
	assignIPv4 := true
	if o.inCfg.IPAM.IPv4NS != nil {
		assignIPv4 = false
		if ns := getK8sNamespace(ctx); ns != "" {
			for _, allowedNS := range o.inCfg.IPAM.IPv4NS {
				if ns == allowedNS {
					assignIPv4 = true
					break
				}
			}
		}
	}

	hostLocalCfg := *o.ipam
	if !assignIPv4 {
		// We must create a hostLocalCfg without IPv4 addressed
		hostLocalCfg.Ranges = nil
		for _, r := range o.ipam.Ranges {
			// hostLocalCfg have been validated so no checks are needed
			ip, _, _ := net.ParseCIDR(r[0].Subnet)
			if ip.To4() != nil {
				continue // IPv4
			}
			hostLocalCfg.Ranges = append(hostLocalCfg.Ranges, r)
			break
		}
	}

	out := cniConfigOut{
		Name:             o.inCfg.Name,
		CNIVersion:       o.inCfg.CNIVersion,
		IsDefaultGateway: o.inCfg.IsDefaultGateway,
		IPAM:             &hostLocalCfg,
	}
	o.trace.Info("To host-local", "config", &out)
	return &out
}

func (o *outIpam) createHostLocalIPAM(
	ctx context.Context, cidrs []string) error {
	o.ipam = &hostLocalIPAM{
		Type:    "host-local",
		DataDir: o.inCfg.IPAM.DataDir,
	}
	for _, cidr := range cidrs {
		r := ranges{rangeItem{Subnet: cidr}}
		o.ipam.Ranges = append(o.ipam.Ranges, r)
	}

	if err := validateHostLocalIPAM(o.ipam); err != nil {
		return err
	}
	return nil
}

func (o *outIpam) writeCache(ctx context.Context) {
	data, err := json.Marshal(o.ipam)
	if err != nil {
		panic(err) // Shouldn't happen
	}
	_ = os.MkdirAll(filepath.Dir(o.cache), 0755)
	if err := os.WriteFile(o.cache, data, 0666); err != nil {
		// It is not a fatal error but can flood the logs, so use debug level
		o.logger.V(1).Error(err, "Write Cache", "file", o.cache)
	}
}

func (o *outIpam) deleteCache() {
	_ = os.Remove(o.cache)
}

func execChained(ctx context.Context, out *cniConfigOut) error {
	// Get the path to the chained ipam
	rawExec := invoke.RawExec{}
	pluginPath, err := rawExec.FindInPath(
		"host-local", filepath.SplitList(os.Getenv("CNI_PATH")))
	if err != nil {
		return err
	}

	// Convert the CniConfigOut to []byte
	stdin, err := json.Marshal(out)
	if err != nil {
		// Should not happen!
		panic(err)
	}

	// Invoke the chained ipam ("host-local")
	res, err := rawExec.ExecPlugin(ctx, pluginPath, stdin, os.Environ())
	if err != nil {
		return err
	}

	// Output the result
	os.Stdout.Write(res)

	// Verbose logging
	logger := logr.FromContextOrDiscard(ctx)
	if l := logger.V(2); l.Enabled() {
		// We don't want to do this unless trace logging is enabled
		resObject := make(map[string]any)
		if err := json.Unmarshal(res, &resObject); err == nil {
			l.Info("execChained result", "stdout", resObject)
		} else {
			// CHECK may not output anything
			if os.Getenv("CNI_COMMAND") != "CHECK" {
				l.Error(err, "Result not json")
			}
		}
	}

	return nil
}

func getOwnNode(ctx context.Context, nodeReader util.NodeReader) (*k8s.Node, error) {
	// If the NODE_NAME environment variable is specified it's assumed
	// to be correct
	if nodeName := os.Getenv("NODE_NAME"); nodeName != "" {
		n, err := nodeReader.GetNode(ctx, nodeName)
		if err != nil {
			return nil, err
		}
		return n, nil
	}

	nodes, err := nodeReader.GetNodes(ctx)
	if err != nil {
		return nil, err
	}
	if n := util.FindOwnNode(ctx, nodes); n != nil {
		return n, nil
	}
	return nil, fmt.Errorf("Own node object not found")
}

// getPodCIDRs Get PodCIDR from the own K8s node object
func getPodCIDRs(
	ctx context.Context, n *k8s.Node, annotation string) ([]string, error) {
	if annotation == "" {
		// No annotation. Get the PodCIDRs from the node.spec
		if n.Spec.PodCIDRs == nil {
			return nil, fmt.Errorf("No spec.podCIDRs found")
		}
		return n.Spec.PodCIDRs, nil
	}

	if c, ok := n.ObjectMeta.Annotations[annotation]; ok {
		podCIDRs := strings.Split(c, ",")
		for _, cidr := range podCIDRs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return nil, err
			}
		}
		return podCIDRs, nil
	}
	return nil, fmt.Errorf("Annotation not found")
}

func getK8sNamespace(ctx context.Context) string {
	// The K8s namespace is found in $CNI_ARGS (or not?)
	if cniArgs := os.Getenv("CNI_ARGS"); cniArgs != "" {
		for _, s := range strings.Split(cniArgs, ";") {
			if v, ok := strings.CutPrefix(s, "K8S_POD_NAMESPACE="); ok {
				return v
			}
		}
	}
	return ""
}

// validateHostLocalIPAM Validates that the type is "host-local" and
// that ranges exists and that the subnets are valid. If 2 subnets are
// specified they must be of different families
func validateHostLocalIPAM(ipam *hostLocalIPAM) error {
	if ipam.Type != "host-local" {
		return fmt.Errorf("Wrong Type")
	}
	if len(ipam.Ranges) == 0 {
		return fmt.Errorf("No Ranges")
	}
	if len(ipam.Ranges) > 2 {
		return fmt.Errorf("Too many Ranges")
	}
	isIPv4 := make([]bool, 0, 2)
	for _, ri := range ipam.Ranges {
		if len(ri) != 1 {
			return fmt.Errorf("Unsupported Range item")
		}
		if ip, _, err := net.ParseCIDR(ri[0].Subnet); err != nil {
			return fmt.Errorf("Invalid subnet %v", err)
		} else {
			isIPv4 = append(isIPv4, ip.To4() != nil)
		}
	}
	if len(isIPv4) == 2 && isIPv4[0] == isIPv4[1] {
		return fmt.Errorf("Subnets of same family")
	}
	return nil
}
