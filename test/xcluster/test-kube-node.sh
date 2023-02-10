#! /bin/sh
##
## kube-node-ipam.sh --
##
##   Help script for Nordix/xcluster-cni/test/kube-node-ipam
##

prg=$(basename $0)
dir=$(dirname $0); dir=$(readlink -f $dir)
me=$dir/$prg
tmp=/tmp/${prg}_$$
test -n "$PREFIX" || PREFIX=fd00:

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
	echo "$prg: $*" >&2
}

## Commands;
##

##   env
##     Print environment.
cmd_env() {

	if test "$cmd" = "env"; then
		set | grep -E '^(__.*)='
		return 0
	fi

	test -n "$xcluster_DOMAIN" || xcluster_DOMAIN=xcluster
	test -n "$XCLUSTER" || die 'Not set [$XCLUSTER]'
	test -x "$XCLUSTER" || die "Not executable [$XCLUSTER]"
	eval $($XCLUSTER env)
}
##   build
##     Build the "kube-node" binary with hard-coded trace logging
cmd_build() {
	$dir/../../build.sh binaries
}

##
## Tests;
##   test [--xterm] [--no-stop] [test] [ovls...] > logfile
##     Exec tests
##
cmd_test() {
	cmd_env
	start=starts
	test "$__xterm" = "yes" && start=start
	rm -f $XCLUSTER_TMP/cdrom.iso

	if test -n "$1"; then
		local t=$1
		shift
		test_$t $@
	else
		test_start
	fi		

	now=$(date +%s)
	tlog "Xcluster test ended. Total time $((now-begin)) sec"
}
##   test start_empty
##     Start empty cluster
test_start_empty() {
	cd $dir
	export xcluster_PREFIX=$PREFIX
	test -n "$__nrouters" || export __nrouters=1
	xcluster_start . $@
	otc 1 check_namespaces
	otc 1 check_nodes
	otcr vip_routes
}
##   test start
##     Start with Multus
test_start() {
	export xcluster_KUBE_NODE_LOG=/var/log/ipam-kube-node.log
	export xcluster_KUBE_NODE_LOG_LEVEL=trace
	test_start_empty multus $@
	otc 1 start_multus
}
##   test start_multilan
##     Start with TOPLOLOGY=multilan-router
test_start_multilan() {
	export TOPOLOGY=multilan-router
	. $($XCLUSTER ovld network-topology)/$TOPOLOGY/Envsettings
	test_start network-topology $@
}
##   test cache
##     Test a with an extra interface and kube-node IPAM with
##     pre-configured cache
test_cache() {
	test_start $@
	otcw create_ipam_cache
	otc 1 start_servers
	xcluster_stop
}
##   test annotation
##     Test a with an extra interface and kube-node IPAM with
##     annotations
test_annotation() {
	test_start $@
	otcw annotate_nodes
	otc 1 start_servers
	xcluster_stop
}
##   test limited_ipv4
##     IPv4 addresses only assigned to PODs in "old-application" NS
test_limited_ipv4() {
	test_start $@
	otcw "annotate_nodes --all-ipv4"
	otc 1 alpine_ipv4
	otc 1 "collect_addresses default"
	otc 1 "check_addresses default ipv6"
	otc 1 "collect_addresses old-application"
	otc 1 "check_addresses old-application dual"
	xcluster_stop
}



##
. $($XCLUSTER ovld test)/default/usr/lib/xctest
indent=''

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
