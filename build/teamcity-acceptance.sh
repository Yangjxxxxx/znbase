#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "${0}")/teamcity-support.sh"

tc_prepare

tc_start_block "Prepare environment for acceptance tests"
# The log files that should be created by -l below can only
# be created if the parent directory already exists. Ensure
# that it exists before running the test.
export TMPDIR=$PWD/artifacts/acceptance
mkdir -p "$TMPDIR"
type=$(go env GOOS)
tc_end_block "Prepare environment for acceptance tests"

tc_start_block "Compile ZNBaseDB"
run pkg/acceptance/prepare.sh
run ln -s znbase-linux-2.6.32-gnu-amd64 znbase  # For the tests that run without Docker.
tc_end_block "Compile ZNBaseDB"

tc_start_block "Compile acceptance tests"
run build/builder.sh mkrelease "$type" -Otarget testbuild TAGS=acceptance PKG=./pkg/acceptance
tc_end_block "Compile acceptance tests"

tc_start_block "Run acceptance tests"
run cd pkg/acceptance
run env TZ=America/New_York \
	stdbuf -eL -oL \
	./acceptance.test -l "$TMPDIR" -test.v -test.timeout 30m 2>&1 \
	| tee "$TMPDIR/acceptance.log" \
	| go-test-teamcity
run cd ../..
tc_end_block "Run acceptance tests"
