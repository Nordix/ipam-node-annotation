#! /bin/sh
##
## kube-node-kind.sh
##
##   Test the kube-node ipam CNI-plugin using KinD.  Nothing is
##   actually executed on KinD, only the node object is read and annotated.
##
## Commands;
##

prg=$(basename $0)
dir=$(dirname $0); dir=$(readlink -f $dir)
tmp=/tmp/${prg}_$$

die() {
    echo "ERROR: $*" >&2
    rm -rf $tmp
    exit 1
}
help() {
    grep '^##' $0 | cut -c3-
    rm -rf $tmp
    exit 0
}
test -n "$1" || help
echo "$1" | grep -qi "^help\|-h" && help

log() {
	echo "$*" >&2
}

kubectl() {
	# Ensure kubeconfig and drop warnings
	$kubectl --kubeconfig=$__kubeconfig $@ 2> /dev/null || die "kubeconfig $@"
}

##  env
##    Print environment.
cmd_env() {
	test "$env_called" = "yes" && return 0
	env_called=yes
	test -n "$__kubeconfig" || __kubeconfig=/tmp/$USER/kubeconfig-kube-node
	export KUBECONFIG=$__kubeconfig
	test -n "$__kind_name" || __kind_name=kube-node-test
	test -n "$__annotation" || __annotation=kube-node.nordix.org/test
	if test "$cmd" = "env"; then
		set | grep -E "^__(kubeconfig|kind_name|annotation)=" | sort
		return 0
	fi
	which kubectl > /dev/null || die "kubectl not executable"
	kubectl=$(which kubectl)
}
##  install_host_local [--dest=/tmp/$USER/bin]
##    Install the "host-local" IPAM CNI-plugin.
##    Prerequisite: A cni-plugins-linux-amd64 is downloaded to ~/Downloads
cmd_install_host_local() {
	test -n "$__dest" || __dest=/tmp/$USER/bin
	mkdir -p $__dest || die "Can't mkdir $__dest"
	local ar=$(find $HOME/Downloads -name "cni-plugins-linux-amd64-*" | sort -rV | head -1)
	test -n "$ar" || die "No archive cni-plugins-linux-amd64 found"
	log "Using $(basename $ar)"
	tar -C $__dest -xf $ar ./host-local
}
##
##  kind_start
##    Start a KinD cluster for tests. Kubeconfig in
##    "/tmp/$USER/kubeconfig-kube-node" by default
cmd_kind_start() {
	cmd_env
	kind create cluster --config $dir/kind.yaml --name $__kind_name
}
##  kind_stop
##    Stop the KinD cluster
cmd_kind_stop() {
	cmd_env
	kind delete cluster --name $__kind_name
}
##  kind_check
##    Checks that the KinD cluster is running
cmd_kind_check() {
	cmd_env
	$kubectl --kubeconfig=$__kubeconfig version > /dev/null 2>&1 \
		|| die "Can't access cluster with kubectl. Is KinD started?"
	mkdir -p $tmp
	local out=$tmp/node
	kubectl get node $__kind_name-control-plane -o json > $out || die "get node"
	cat $out | jq .spec.podCIDRs | grep -q null && die "No podCIDRs"
}

# -------------------------------------------------------------------
# Test help-functions

# Should be called in each test-case
cmd_test_prep() {
	cmd_env
	export CNI_COMMAND=ADD
	export CNI_CONTAINERID=Container1
	export CNI_NETNS=None
	export CNI_IFNAME=None
	export CNI_ARGS="IgnoreUnknown=1;K8S_POD_NAMESPACE=default"
	export KUBECONFIG=$__kubeconfig

	rm -rf $tmp
	mkdir -p $tmp
	cfg=$tmp/cfg.json
	res=$tmp/result.json
	out=$tmp/out

	test "$test_prep_called" = "yes" && return 0
	test_prep_called=yes

	cmd_kind_check
	kube_node=$(readlink -f $dir/../../_output/kube-node)
	test -x $kube_node || die "Not executable [$kube_node]"
	if test -z "$__host_local"; then
		if which host-local > /dev/null; then
			__host_local=$(which host-local)
		elif test -x /opt/cni/bin/host-local; then
			__host_local=/opt/cni/bin/host-local
		elif test -x $HOME/bin/host-local; then
			__host_local=$HOME/bin/host-local
		else
			__host_local=/tmp/$USER/bin/host-local
		fi
	fi
	test -x $__host_local || die "Can't find host-local"
	export CNI_PATH=$(dirname $__host_local)
	export NODE_NAME=$__kind_name-control-plane
	if test "$__log" = "yes"; then
		rm -f /tmp/kube-node-log
		logcfg='"loglevel": "trace", "logfile": "/tmp/kube-node-log",'
	fi
	annotate
}
# Invoke 
invoke() {
	test "$__v" = "yes" && cat $cfg | jq
	rc=0
	cat $cfg | $kube_node > $res || rc=$?
}
# check_error [expected]
#   An unexpected error or an OK when an error is expected is a FAILURE
check_error() {
	test -r $res || die "No result file"
	test -s $res || die "Result file is empty"
	if cat $res | jq .code | grep -q null; then
		# No error
		if test "$1" = "expected"; then
			cat $res | jq
			die "Expected error but got OK result"
		fi
	else
		# We have got an error
		if test "$1" != "expected"; then
			cat $res | jq
			die "Unexpected error"
		fi
	fi
	test "$__v" = "yes" && cat $res | jq
}
# Check dual stack addresses
check_dual() {
	cat $res | jq .ips | grep -q null && die "No ips in result"
	test $(cat $res | jq '.ips|length') -eq 2 || die "Not 2 ips"
	cat $res | jq '.ips[].address' | grep -qF . || die "IPv4 missing"
	cat $res | jq '.ips[].address' | grep -qF : || die "IPv6 missing"
}
# Check IPv6 address (one)
check_ipv6() {
	cat $res | jq .ips | grep -q null && die "No ips in result"
	test $(cat $res | jq '.ips|length') -eq 1 || die "Not 1 ips"
	cat $res | jq '.ips[].address' | grep -qF : || die "IPv6 missing"
	cat $res | jq '.ips[].address' | grep -qF . && die "IPv4 present"
}
# Check IPv4 address (one)
check_ipv4() {
	cat $res | jq .ips | grep -q null && die "No ips in result"
	test $(cat $res | jq '.ips|length') -eq 1 || die "Not 1 ips"
	cat $res | jq '.ips[].address' | grep -qF : && die "IPv6 present"
	cat $res | jq '.ips[].address' | grep -qF . || die "IPv4 missing"
}
# annotate [subnets]
annotate() {
	# Clear the annotation always
	kubectl --kubeconfig=$__kubeconfig annotate node \
		$__kind_name-control-plane ${__annotation}- > /dev/null

	test -n "$1" || return 0
	kubectl --kubeconfig=$__kubeconfig annotate node \
		$__kind_name-control-plane $__annotation="$1" > /dev/null
}

# -------------------------------------------------------------------
##
## Test-cases: Requires kind_start. Use "--v" to print the json result

##  version
##    Check the CNI version. This the most basic test
cmd_version() {
	cmd_test_prep
	log "Test-case: Check the CNI version"
	export CNI_COMMAND=VERSION
	echo "{}" > $cfg
	invoke
	check_error
	cat $res | jq .supportedVersions | grep -q null && die "No supportedVersions"
	local nver=$(cat $res | jq -r '.supportedVersions|length')
}
##  empty_input
##    An empty input is only valid for CNI_COMMAND=VERSION
cmd_empty_input() {
	cmd_test_prep
	log "Test-case: An empty input is only valid for CNI_COMMAND=VERSION"
	echo "{}" > $cfg
	invoke
	check_error expected
}
##  k8s_addresses
##    Get K8s addresses. The normal K8s case. Use $KUBECONFIG
cmd_k8s_addresses() {
	cmd_test_prep
	log "Test-case: Get K8s addresses"
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "isDefaultGateway": true,
  "ipam": {
    "type": "kube-node",
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	invoke
	check_error
	check_dual
}
##  no_kubeconfig
##    Try to get K8s addresses without $KUBECONFIG
cmd_no_kubeconfig() {
	cmd_test_prep
	log 'Test-case: Try to get K8s addresses without $KUBECONFIG'
	unset KUBECONFIG
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "isDefaultGateway": true,
  "ipam": {
    "type": "kube-node",
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	invoke
	check_error expected
}
##  kubeconfig_in_spec
##    Try to get K8s addresses with specified kubeconfig
cmd_kubeconfig_in_spec() {
	cmd_test_prep
	log 'Test-case: Try to get K8s addresses with specified kubeconfig'
	export KUBECONFIG=/path/to/nowhere
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "isDefaultGateway": true,
  "ipam": {
    "type": "kube-node",
    "kubeconfig": "$__kubeconfig",
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	invoke
	check_error
	check_dual
}
##  limit_ipv4
##    Limit the IPv4 assignment
cmd_limit_ipv4() {
	cmd_test_prep
	log "Test-case: Limit the IPv4 assignment"
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "isDefaultGateway": true,
  "ipam": {
    "type": "kube-node",
    "ipv4-namespaces": [
        "old-application"
    ],
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	invoke
	check_error
	check_ipv6

	export CNI_ARGS="IgnoreUnknown=1;K8S_POD_NAMESPACE=old-application"
	export CNI_CONTAINERID=Container2
	invoke
	check_error
	check_dual
}
##  annotation_dual
##    Assign dual-stack addresses using an annotation
cmd_annotation_dual() {
	cmd_test_prep
	log "Test-case: Assign dual-stack addresses using an annotation"
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "ipam": {
    "type": "kube-node",
    "annotation": "$__annotation",
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	annotate "fc00:400::/120,172.20.4.0/24"
	invoke
	check_error
	check_dual	
}
##  annotation_ipv6
##    Assign an ipv6 address using an annotation
cmd_annotation_ipv6() {
	cmd_test_prep
	log "Test-case: Assign an ipv6 address using an annotation"
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "ipam": {
    "type": "kube-node",
    "annotation": "$__annotation",
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	annotate "fc00:400::/120"
	invoke
	check_error
	check_ipv6
}
##  annotation_ipv4
##    Assign an ipv4 address using an annotation
cmd_annotation_ipv4() {
	cmd_test_prep
	log "Test-case: Assign an ipv4 address using an annotation"
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "ipam": {
    "type": "kube-node",
    "annotation": "$__annotation",
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	annotate "172.20.4.0/24"
	invoke
	check_error
	check_ipv4
}
##  annotation_too_many
##    Too many subnets in annotation
cmd_annotation_to_long() {
	cmd_test_prep
	log "Test-case: Too many subnets in annotation"
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "ipam": {
    "type": "kube-node",
    "annotation": "$__annotation",
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	annotate "fc00:400::/120,172.20.4.0/24,fd00:500::/64"
	invoke
	check_error expected
}
##  annotation_invalid_subnet
##    Invalid subnet in annotation
cmd_annotation_invalid_subnet() {
	cmd_test_prep
	log "Test-case: Invalid subnet in annotation"
	cat > $cfg <<EOF
{
  "name": "k8snet",
  "cniVersion": "1.0.0",
  "ipam": {
    "type": "kube-node",
    "annotation": "$__annotation",
    $logcfg
    "dataDir": "$tmp"
  }
}
EOF
	annotate "fc00:400::/120,172.20.4.0/120"
	invoke
	check_error expected
}

##  test_all
##    Execute all test cases
cmd_test_all() {
	cmd_test_prep
	log "Testing kube-node $($kube_node -version)"
	cmd_version
	cmd_empty_input
	cmd_k8s_addresses
	cmd_no_kubeconfig
	cmd_kubeconfig_in_spec
	cmd_limit_ipv4
	cmd_annotation_dual
	cmd_annotation_ipv6
	cmd_annotation_ipv4
	cmd_annotation_to_long
	cmd_annotation_invalid_subnet
}

##
# Get the command
cmd=$1
shift
grep -q "^cmd_$cmd()" $0 $hook || die "Invalid command [$cmd]"

while echo "$1" | grep -q '^--'; do
    if echo $1 | grep -q =; then
	o=$(echo "$1" | cut -d= -f1 | sed -e 's,-,_,g')
	v=$(echo "$1" | cut -d= -f2-)
	eval "$o=\"$v\""
    else
	o=$(echo "$1" | sed -e 's,-,_,g')
	eval "$o=yes"
    fi
    shift
done
unset o v
long_opts=`set | grep '^__' | cut -d= -f1`

# Execute command
trap "die Interrupted" INT TERM
cmd_$cmd "$@"
status=$?
rm -rf $tmp
exit $status
