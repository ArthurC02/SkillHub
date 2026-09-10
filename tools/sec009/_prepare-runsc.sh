# Shared in-container preparation for the sandbox test scripts in this
# directory. Not executable on its own: callers cat it in front of their own
# body and hand the whole thing to bash inside a privileged container.
set -e

# `</dev/null` on every apt-get: the callers pipe this whole script into
# `bash -s`, so both bash and apt are reading from the same stdin, and without
# this apt silently consumes the rest of the script.
if ! command -v runsc >/dev/null 2>&1; then
  apt-get update -qq >/dev/null 2>&1 </dev/null
  apt-get install -y -qq curl ca-certificates >/dev/null 2>&1 </dev/null

  ARCH=$(uname -m)
  URL=https://storage.googleapis.com/gvisor/releases/release/latest/${ARCH}
  curl -fsSL -o /usr/local/bin/runsc "${URL}/runsc"
  curl -fsSL -o /tmp/runsc.sha512 "${URL}/runsc.sha512"
  ( cd /usr/local/bin && sha512sum -c /tmp/runsc.sha512 --ignore-missing )
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
