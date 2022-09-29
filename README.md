# IPAM CNI-plugin - node-annotation

The `node-annotation` is an IPAM [CNI-plugin](
https://github.com/containernetworking/cni) that reads the addresses
from an annotation in the Kubernetes [node object](
https://kubernetes.io/docs/concepts/architecture/nodes/).

Example:
```json
{
    "name": "mynet",
    "type": "ipvlan",
    "master": "eth0",
    "ipam": {
        "type": "node-annotation",
        "annotation": "example.com/ipvlan-ranges"
    }
}
```
And in the node object:
```
# kubectl get node $(hostname) -o json | jq .metadata.annotations
{
  "example.com/ipvlan-ranges": "ranges: [[{\"subnet\": \"172.16.5.0/24\"}]]",
  "node.alpha.kubernetes.io/ttl": "0",
  "volumes.kubernetes.io/controller-managed-attach-detach": "true"
}
```

The `node-annotation` ipam is a wrapper for the [host-local](
https://www.cni.dev/plugins/current/ipam/host-local/) ipam. *Anything*
in the annotation will be inserted in a `host-local` config. The
example above will result in;

```json
{
  "ipam": {
    "type": "host-local",
    "ranges": [[{"subnet": "172.16.5.0/24"}]]
  }
}
```
This gives you the freedom to use any `host-local` configuration.

For now `node-annotation` is implemented as a shell script. It is
intended mainly for testing, but if it's consider useful it can be
rewritten in `go` and be made more rubust an effective. PR's are welcome.


## Usage

`node-annotation` shall be installed in the cbi-bin directory, usually
"/opt/cni/bin". `node-annotation` must be able to get the K8s node
objects using `kubectl get nodes -o json` and analyze with [jq](
https://stedolan.github.io/jq/)

Configuration is in `json` format and is read from
`/etc/cni/node-annotation.conf` or `$NODE_ANNOTATION_CFG`. Example;

```json
{
   "kubeconfig": "/etc/kubernetes/kubeconfig",
   "nextipam": "/opt/cgi/bin/host-local"
}
```

`kubeconfig` is needed unless `$KUBECONFIG` is defined. `nextipam` is
optional.

**NOTE**; a "key" must only contain characters that are valid in a
  shell script variable. That means no dash (-).



## Manual Testing

The `node-annotation` script can be invoked with a parameter to test
some things on a cluster.

```
# /opt/cni/bin/node-annotation -h

 node-annotation --

   An IPAM CNI-plugin that uses annotations on the K8s node object
   https://github.com/Nordix/ipam-node-annotation

 Commands;

   ipam
     Act as an ipam CNI-plugin. This is the default command
   error_quit [msg]
     Print an error in standard CNI json format and quit
   my_node
     Print the own node object
   get_annotation [--node=node] <annotation>
     Print the value of the annotation in the K8s node object

# kubectl annotate node $(hostname) example.com/ipvlan-ranges="\"ranges\": [
  { \"subnet\": \"4000::16.0.0.0/120\" },
  { \"subnet\": \"16.0.0.0/24\" }
]"
# /opt/cni/bin/node-annotation get_annotation example.com/ipvlan-ranges
"ranges": [
  [{ "subnet": "4000::16.0.0.0/120" }],
  [{ "subnet": "16.0.0.0/24" }]
]
```

A test of the entire function;
```
# cat > test.cfg <<EOF
{
    "name": "mynet",
    "type": "ipvlan",
    "master": "eth0",
    "ipam": {
        "type": "node-annotation",
        "annotation": "example.com/ipvlan-ranges"
    }
}
EOF
# export NODE_ANNOTATION_CFG=./nacfg
# cat > $NODE_ANNOTATION_CFG <<EOF
{
  "nextipam": "cat"
}
EOF
cat test.cfg | /opt/cni/bin/node-annotation
{
  "name": "mynet",
  "type": "ipvlan",
  "master": "eth0",
  "ipam": {
    "type": "cat",
    "ranges": [
      {
        "subnet": "4000::16.0.0.0/120"
      },
      {
        "subnet": "16.0.0.0/24"
      }
    ]
  }
}
```


## Kind

If you have [Multus](https://github.com/k8snetworkplumbingwg/multus-cni)
running in [kind](https://kind.sigs.k8s.io/) there are some steps to make
`node-annotation` work.

Only the controller have a kubeconfig at "/etc/kubernetes/admin.conf".
Create one at the workers with and configure `node-annotation` to use it;

```
kind get kubeconfig --name --internal | \
  docker exec -i worker tee /etc/kubernetes/kubeconfig > /dev/null
echo "{ \"kubeconfig\": \"/etc/kubernetes/kubeconfig\" }" | docker exec -i worker \
  tee /etc/cni/node-annotation.conf > /dev/null
# (repeat for all workers)
```

The Multus kubeconfig (/etc/cni/net.d/multus.d/multus.kubeconfig)
can't be used due to restrictions.

Now you can annotate the workers and create a NAD. Example;
```
kubectl annotate node worker example.com/bridge="\"ranges\": [
  [{ \"subnet\":\"4000::16.0.0.0/120\", \"rangeStart\":\"4000::16.0.0.0\" , \"rangeEnd\":\"4000::16.0.0.7\"}],
  [{ \"subnet\":\"16.0.0.0/24\", \"rangeStart\":\"16.0.0.0\" , \"rangeEnd\":\"16.0.0.7\"}]
]"
cat | kubectl apply -f - <<EOF
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: meridio-bridge
spec:
  config: '{
    "cniVersion": "0.4.0",
    "type": "bridge",
    "bridge": "cbr2",
    "isGateway": true,
    "ipam": {
      "type": "node-annotation",
      "annotation": "example.com/bridge"
    }
  }'
EOF
```
