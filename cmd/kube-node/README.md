# Nordix xcluster-cni/kube-node

An IPAM plugin that reads CIDRs from the K8s node object and calls
[host-local](https://www.cni.dev/plugins/current/ipam/host-local/)

```json
{
  "name": "my-net",
  "cniVersion": "1.0.0",
  "isDefaultGateway": true,
  "ipam": {
    "type": "kube-node",
    "dataDir": "/run/container-ipam-state/my-net",
    "annotation": "example.com/my-net-cidrs",
    "ipv4-namespaces": [
        "old-application"
    ]
  }
}
```

* dataDir - optional. Passed to `host-local`. The `host-local`
  configuration is also cached in this directory. This should be a
  directory that is cleared on node reboot! Otherwise you will leak
  addresses

* annotation - optional. If defined the CIDR configuration passed to
  `host-local` is taken from this annotation. If unspecified, K8s
  `spec.podCIDRs` are used

* ipv4-namespaces - optional. If defined, only PODs in these
  namespaces will be assigned IPv4 addresses


The annotation is set individually for each node:
```
kubectl annotate node worker-1 example.com/my-net-cidrs="198.51.100.0/26,2001:DB8::/112"
kubectl annotate node worker-10 example.com/my-net-cidrs="2001:DB8::1:0/112"
```

The example shows that PODs on `worker-10` will be IPv6-only, which
saves IPv4 ranges but requires affinity for IPv4 PODs.

