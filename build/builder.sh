#!/usr/bin/env bash

set -euo pipefail

image=znbasedb/builder
version=20210510-114300

function init() {
  docker build --tag="${image}" "$(dirname "${0}")/builder"
}

if [ "${1-}" = "pull" ]; then
  docker pull "${image}:${version}"
  exit 0
fi

if [ "${1-}" = "init" ]; then
  init
  exit 0
fi

if [ "${1-}" = "push" ]; then
  init
  tag=$(date +%Y%m%d-%H%M%S)
  docker tag "${image}" "${image}:${tag}"
  docker push "${image}:${tag}"
  exit 0
fi

if [ "${1-}" = "version" ]; then
  echo "${version}"
  exit 0
fi

cached_volume_mode=
delegated_volume_mode=
if [ "$(uname)" = "Darwin" ]; then
  # This boosts filesystem performance on macOS at the cost of some consistency
  # guarantees that are usually unnecessary in development.
  # For details: https://docs.docker.com/docker-for-mac/osxfs-caching/
  delegated_volume_mode=:delegated
  cached_volume_mode=:cached
fi

GOROOT=$(go env GOROOT)
GOPATH=$(go env GOPATH)
gopath0=${GOPATH%%:*}
# 升级cache1解决udr不兼容c库问题
gocache=${GOCACHEPATH1-$gopath0}

if [ -t 0 ]; then
  tty=--tty
fi

# Absolute path to the toplevel znbase directory.
znbase_toplevel=$(dirname "$(cd "$(dirname "${0}")"; pwd)")

# Ensure the artifact sub-directory always exists and redirect
# temporary file creation to it, so that CI always picks up temp files
# (including stray log files).
mkdir -p "${znbase_toplevel}"/artifacts
export TMPDIR=$znbase_toplevel/artifacts

# We'll mount a fresh directory owned by the invoking user as the container
# user's home directory because various utilities (e.g. bash writing to
# .bash_history) need to be able to write to there.
container_home=/home/roach
host_home=${znbase_toplevel}/build/builder_home
mkdir -p "${host_home}"

# Since we're mounting both /root and its subdirectories in our container,
# Docker will create the subdirectories on the host side under the directory
# that we're mounting as /root, as the root user. This creates problems for CI
# processes trying to clean up the working directory, so we create them here
# as the invoking user to avoid root-owned paths.
#
# Note: this only happens on Linux. On Docker for Mac, the directories are
# still created, but they're owned by the invoking user already. See
# https://github.com/docker/docker/issues/26051.
mkdir -p "${host_home}"/.yarn-cache

# Run our build container with a set of volumes mounted that will
# allow the container to store persistent build data on the host
# computer.
#
# This script supports both circleci and development hosts, so it must
# support cases where the architecture inside the container is
# different from that outside the container. We can map /src/ directly
# into the container because it is architecture-independent. We then
# map certain subdirectories of ${GOPATH}/pkg into both ${GOPATH}/pkg
# and ${GOROOT}/pkg. The ${GOROOT} mapping is needed so they can be
# used to cache builds of the standard library. /bin/ is mapped
# separately to avoid clobbering the host's binaries. Note that the
# path used for the /bin/ mapping is also used in the defaultBinary
# function of localcluster.go.
#
# We always map the znbase source directory that contains this script into
# the container's $GOPATH/src. By default, we also mount the host's $GOPATH/src
# directory to the container's $GOPATH/src. That behavior can be turned off by
# setting BUILDER_HIDE_GOPATH_SRC to 1, which results in only the znbase
# source code (and its vendored dependencies) being available within the
# container. This setting is useful to prevent missing vendored dependencies
# from being accidentally resolved to the hosts's copy of those dependencies.

# Ensure that all directories to which the container must be able to write are
# created as the invoking user. Docker would otherwise create them when
# mounting, but that would deny write access to the invoking user since docker
# runs as root.

vols=
# It would be cool to interact with Docker from inside the builder, but the
# socket is owned by root, and our unpriviledged user can't access it. If we
# could make this work, we could run our acceptance tests from inside the
# builder, which would be cleaner and simpler than what we do now (which is to
# build static binaries in the container and then run them on the host).
#
# vols="${vols} --volume=/var/run/docker.sock:/var/run/docker.sock"
vols="${vols} --volume=${host_home}:${container_home}${cached_volume_mode}"

mkdir -p "${HOME}"/.yarn-cache
vols="${vols} --volume=${HOME}/.yarn-cache:${container_home}/.yarn-cache${cached_volume_mode}"

# If we're running in an environment that's using git alternates, like TeamCity,
# we must mount the path to the real git objects for git to work in the container.
alternates_file=${znbase_toplevel}/.git/objects/info/alternates
if test -e "${alternates_file}"; then
  alternates_path=$(cat "${alternates_file}")
  vols="${vols} --volume=${alternates_path}:${alternates_path}${cached_volume_mode}"
fi

backtrace_dir=${znbase_toplevel}/../../znbaselabs/backtrace
if test -d "${backtrace_dir}"; then
  vols="${vols} --volume=${backtrace_dir}:/opt/backtrace${cached_volume_mode}"
  vols="${vols} --volume=${backtrace_dir}/znbase.cf:${container_home}/.coroner.cf${cached_volume_mode}"
fi

if [ "${BUILDER_HIDE_GOPATH_SRC:-}" != "1" ]; then
  vols="${vols} --volume=${gopath0}/src:/go/src${cached_volume_mode}"
fi
vols="${vols} --volume=${znbase_toplevel}:/go/src/github.com/znbasedb/znbase${cached_volume_mode}"
vols="${vols} --volume=${GOROOT}:/usr/local/go"

# If ${znbase_toplevel}/bin doesn't exist on the host, Docker creates it as
# root unless it already exists. Create it first as the invoking user.
# (This is a bug in the Docker daemon that only occurs when bind-mounted volumes
# are nested, as they are here.)
mkdir -p "${znbase_toplevel}"/bin{.docker_amd64,}
vols="${vols} --volume=${znbase_toplevel}/bin.docker_amd64:/go/src/github.com/znbasedb/znbase/bin${delegated_volume_mode}"

mkdir -p "${gocache}"/docker/bin
vols="${vols} --volume=${gocache}/docker/bin:/go/bin${delegated_volume_mode}"
mkdir -p "${gocache}"/docker/native
vols="${vols} --volume=${gocache}/native:/go/native${delegated_volume_mode}"
mkdir -p "${gocache}"/docker/pkg
vols="${vols} --volume=${gocache}/docker/pkg:/go/pkg${delegated_volume_mode}"
mkdir -p "${gocache}"/docker/data
vols="${vols} --volume=${gocache}/docker/data:/data"

# Attempt to run in the container with the same UID/GID as we have on the host,
# as this results in the correct permissions on files created in the shared
# volumes. This isn't always possible, however, as IDs less than 100 are
# reserved by Debian, and IDs in the low 100s are dynamically assigned to
# various system users and groups. To be safe, if we see a UID/GID less than
# 500, promote it to 501. This is notably necessary on macOS Lion and later,
# where administrator accounts are created with a GID of 20. This solution is
# not foolproof, but it works well in practice.
uid=$(id -u)
gid=$(id -g)
[ "$uid" -lt 500 ] && uid=501
[ "$gid" -lt 500 ] && gid=$uid

# -i causes some commands (including `git diff`) to attempt to use
# a pager, so we override $PAGER to disable.

# shellcheck disable=SC2086
docker run --init --privileged -i ${tty-} --rm \
  -u "$uid:$gid" \
  ${vols} \
  --workdir="/go/src/github.com/znbasedb/znbase" \
  --env="TMPDIR=/go/src/github.com/znbasedb/znbase/artifacts" \
  --env="PAGER=cat" \
  --env="CC=gcc" \
  --env="GOTRACEBACK=${GOTRACEBACK-all}" \
  --env="TZ=America/New_York" \
  --env=ZNBASE_BUILDER_CCACHE \
  --env=ZNBASE_BUILDER_CCACHE_MAXSIZE \
  "${image}:${version}" "$@"
