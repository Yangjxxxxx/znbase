#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "${0}")/teamcity-support.sh"

tc_start_block "Prepare environment"
# Grab a testing license good for one hour.
ZNBASE_DEV_LICENSE=$(curl -f "https://register.znbasedb.com/api/prodtest")
run mkdir -p artifacts
maybe_ccache
tc_end_block "Prepare environment"

tc_start_block "Compile ZNBaseDB"
run build/builder.sh make build
tc_end_block "Compile ZNBaseDB"

tc_start_block "Compile roachprod/workload/roachtest"
run build/builder.sh make bin/roachprod bin/workload bin/roachtest
tc_end_block "Compile roachprod/workload/roachtest"

tc_start_block "Run local roachtests"
# TODO(peter,dan): curate a suite of the tests that works locally.
run build/builder.sh env \
  ZNBASE_DEV_LICENSE="$ZNBASE_DEV_LICENSE" \
	stdbuf -oL -eL \
	./bin/roachtest run '(acceptance|kv/splits|cdc/bank)' \
  --local \
  --znbase "znbase" \
  --roachprod "bin/roachprod" \
  --workload "bin/workload" \
  --artifacts artifacts \
  --teamcity 2>&1 \
	| tee artifacts/roachtest.log
tc_end_block "Run local roachtests"
