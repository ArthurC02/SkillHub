# Shared in-container preparation for the sandbox test scripts in this
# directory. Not executable on its own: callers cat it in front of their own
# body and hand the whole thing to bash inside a privileged container.
# shellcheck shell=bash
set -e

# `</dev/null` on every apt-get: the callers pipe this whole script into
# `bash -s`, so both bash and apt are reading from the same stdin, and without
# this apt silently consumes the rest of the script.
if ! command -v runsc >/dev/null 2>&1; then
  apt-get update -qq >/dev/null 2>&1 </dev/null
  apt-get install -y -qq curl ca-certificates zstd >/dev/null 2>&1 </dev/null

  ARCH=$(uname -m)
  # The version comes from the caller, which reads the same baseline file node
  # admission compares against: a probe run against any other build measures a
  # binary no Run will ever execute. `latest` no longer serves these files.
  : "${SEC009_RUNSC_VERSION:?the calling script must pass the gVisor baseline}"
  URL=https://storage.googleapis.com/gvisor/releases/release/${SEC009_RUNSC_VERSION#release-}/${ARCH}
  curl -fsSL -o /tmp/gvisor.tar.zstd "${URL}/gvisor.tar.zstd"
  : "${SEC009_RUNSC_SHA512:?the calling script must pass the pinned sha512}"
  echo "${SEC009_RUNSC_SHA512}  /tmp/gvisor.tar.zstd" | sha512sum -c -
  tar --zstd -xf /tmp/gvisor.tar.zstd -C /usr/local/bin
  rm -f /tmp/gvisor.tar.zstd
fi

# Empties the root cgroup so subtree_control becomes writable: cgroup v2's
# no-internal-process rule blocks delegation while processes sit directly in
# it, which is where a container's root cgroup starts.
if [ ! -d /sys/fs/cgroup/init ]; then
  mkdir -p /sys/fs/cgroup/init
  mapfile -t root_cgroup_procs < /sys/fs/cgroup/cgroup.procs
  for p in "${root_cgroup_procs[@]}"; do
    echo "$p" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
  done
  echo "+cpu +memory +pids" > /sys/fs/cgroup/cgroup.subtree_control
fi

# shellcheck disable=SC2034 # read by the body each caller appends
RUNSC="runsc --platform=systrap --network=none do"
