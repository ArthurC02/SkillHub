# Shared in-container preparation for the sandbox test scripts in this
# directory. Not executable on its own: callers cat it in front of their own
# body and hand the whole thing to bash inside a privileged container.
set -e

# `</dev/null` on every apt-get: the callers pipe this whole script into
# `bash -s`, so both bash and apt are reading from the same stdin, and without
# this apt silently consumes the rest of the script.
if ! command -v runsc >/dev/null 2>&1; then
  apt-get update -qq >/dev/null 2>&1 </dev/null
  apt-get install -y -qq curl ca-certificates bzip2 >/dev/null 2>&1 </dev/null

  ARCH=$(uname -m)
  URL=https://storage.googleapis.com/gvisor/releases/release/latest/${ARCH}
  # The release ships the binary beside a gvisor-bin/ directory it loads at
  # run time, so both land in the same directory on PATH; a lone runsc
  # installs cleanly and then fails to start a sandbox.
  curl -fsSL -o /tmp/gvisor.tar.bz2 "${URL}/gvisor.tar.bz2"
  curl -fsSL -o /tmp/gvisor.tar.bz2.sha512 "${URL}/gvisor.tar.bz2.sha512"
  ( cd /tmp && sha512sum -c gvisor.tar.bz2.sha512 --ignore-missing )
  tar xjf /tmp/gvisor.tar.bz2 -C /usr/local/bin runsc gvisor-bin
  chmod 755 /usr/local/bin/runsc
fi

# Empties the root cgroup so subtree_control becomes writable: cgroup v2's
# no-internal-process rule blocks delegation while processes sit directly in
# it, which is where a container's root cgroup starts.
if [ ! -d /sys/fs/cgroup/init ]; then
  mkdir -p /sys/fs/cgroup/init
  for p in $(cat /sys/fs/cgroup/cgroup.procs); do
    echo "$p" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
  done
  echo "+cpu +memory +pids" > /sys/fs/cgroup/cgroup.subtree_control
fi

RUNSC="runsc --platform=systrap --network=none do"
