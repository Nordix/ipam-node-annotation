package util

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"bufio"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"github.com/Nordix/ipam-node-annotation/pkg/log"
	"github.com/go-logr/logr"
)

// GetClientset Returns a Clientset fabricated in the "standard" way
// (as close as it gets anyway). The function works both in a POD or
// anyway a kubeconfig is accessible
func GetClientset() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig :=
			clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(config)
}
// GetApi Return a Core API or die trying
func GetApi(ctx context.Context) core.CoreV1Interface {
	clientset, err := GetClientset()
	if err != nil {
		log.Fatal(ctx, "Get clientset", "error", err)
	}
	return clientset.CoreV1()
}

// EmitJson Emit an object in json format on stdout
func EmitJson(object any) {
	if s, err := json.Marshal(object); err == nil {
		fmt.Println(string(s))
	}
}

// Find own node.  The own node is found by comparing
// status.nodeInfo.machineID with the "/etc/machine-id" file. The node
// name may differ from the hostname and several nodes may have the
// same hostname so this is the (only?) safe way
func FindOwnNode(ctx context.Context, nodes []k8s.Node) *k8s.Node {
	logger := logr.FromContextOrDiscard(ctx)
	// First try the machine-id
	file, err := os.Open("/etc/machine-id")
	if err != nil {
		logger.Error(err, "os.Open", "file", "/etc/machine-id")
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var machineId string
	for scanner.Scan() {
		machineId = scanner.Text()
		if machineId == "" {
			// Find first non-empty line (may be only white-space though...)
			continue
		}
		logger.V(2).Info("Read machine-id", "machine-id", machineId)
		if machineId != "" {
			for _, n := range nodes {
				if n.Status.NodeInfo.MachineID == machineId {
					logger.V(2).Info(
						"Found own node", "name", n.ObjectMeta.Name)
					return &n
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Error(err, "Scanning machine-id")
	}
	return nil
}

// FindNode Returns the named node or nil
func FindNode(ctx context.Context, nodes []k8s.Node, name string) *k8s.Node {
	for _, n := range nodes {
		if n.ObjectMeta.Name == name {
			return &n
		}
	}
	return nil
}

// NodeReader Interface to simplify unit-test
type NodeReader interface {
	GetNodes(ctx context.Context) ([]k8s.Node, error)
	GetNode(ctx context.Context, name string) (*k8s.Node, error)
}
type realNodeReader struct{}

func RealNodeReader() NodeReader {
	return &realNodeReader{}
}

// GetNodes Returns all node objects
func (o *realNodeReader) GetNodes(ctx context.Context) ([]k8s.Node, error) {
	logger := logr.FromContextOrDiscard(ctx)
	api := GetApi(ctx)

	nodes, err := api.Nodes().List(ctx, meta.ListOptions{})
	if err != nil {
		return nil, err
	}
	logger.V(2).Info("Read nodes", "count", len(nodes.Items))
	return nodes.Items, nil
}

// GetNode Reads and returns a node object. Only one object is read, making
// this more efficient than call GetNodes() and FindNode()
func (o *realNodeReader) GetNode(ctx context.Context, name string) (*k8s.Node, error) {
	if name == "" {
		return nil, fmt.Errorf("No name")
	}

	// Read just the selected node
	api := GetApi(ctx)
	nodes, err := api.Nodes().List(ctx, meta.ListOptions{
		FieldSelector: "metadata.name=" + name,
	})
	if err != nil {
		return nil, err
	}
	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("Node not found")
	}
	return &nodes.Items[0], nil
}

// CniVersion Holds the CNI version. This variable MUST be updated to
// the CNI version in the request after it has been read from stdin.
var CniVersion = "0.1.0"

// CniError Returns a CNI formatted error message
func CniError(ctx context.Context, err error, code uint, msg string) string {
	cnierr := struct {
		CniVersion string `json:"cniVersion"`
		Code uint `json:"code"`
		Msg string `json:"msg"`
		Details string `json:"details"`
	}{
		CniVersion: CniVersion,
		Code: code,
		Msg: msg,
		Details: err.Error(),
	}
	out, err := json.Marshal(cnierr)
	if err != nil {
		return fmt.Sprintf(
			`{"cniVersion":"%s","code": %d,"msg":"%s"}`, CniVersion, code, msg)
	}
	return string(out)
}

// CniErrorEmit Emits a CNI formatted error on stdout and exit
func CniErrorExit(ctx context.Context, err error, code uint, msg string) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Error(err, "CniErrorEmit", "code", code, "msg", msg)
	fmt.Println(CniError(ctx, err, code, msg))
	os.Exit(1)
}
