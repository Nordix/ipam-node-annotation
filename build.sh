#! /bin/sh
##
## build.sh --
##   Build xcluster-cni.
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
	echo "$prg: $*" >&2
}
dbg() {
	test -n "$__verbose" && echo "$prg: $*" >&2
}
findf() {
	f=$ARCHIVE/$1
	test -r $f || f=$HOME/Downloads/$1
	test -r $f || die "Not found [$f]"
}

##  env
##    Print environment.
##
cmd_env() {
	test "$cmd" = "env" && set | grep -E '^(__.*)='
}
##  binaries [--version=]
##    Build binaries in ./_output
cmd_binaries() {
	cmd_env
	cd $dir
	mkdir -p _output
	test -n "$__version" || __version=$(date +%Y.%m.%d-%H.%M.%S)
	CGO_ENABLED=0 GOOS=linux go build \
		-ldflags "-extldflags '-static' -X main.version=$__version $LDFLAGS" \
		-o _output ./cmd/... || die "go build"
	strip _output/*
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
