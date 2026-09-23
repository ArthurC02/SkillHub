#!/usr/bin/env bash
# Hands this shell a cgroup v2 subtree of its own, which a login session gets
# from systemd and a hosted runner's job cgroup does not, then execs into it.
# Usage: bash tools/ci/with-delegated-cgroup.sh <command> [args...]
set -euo pipefail

root=/sys/fs/cgroup
slice="$root/skillhub.slice"

if [ ! -d "$root" ] || [ ! -f "$root/cgroup.controllers" ]; then
	echo "with-delegated-cgroup: no cgroup v2 at $root" >&2
	exit 1
fi

sudo mkdir -p "$slice/session"
# The root cgroup is exempt from the rule that a cgroup holding processes
# cannot hand controllers to its children.
sudo sh -c "printf '+memory +pids' > '$root/cgroup.subtree_control'"
sudo sh -c "printf '+memory +pids' > '$slice/cgroup.subtree_control'"
sudo chown -R "$(id -u):$(id -g)" "$slice"
# Moving a process needs write access to the common ancestor as well, and the
# job's own cgroup belongs to root, so this one write stays privileged.
sudo sh -c "printf '%s' $$ > '$slice/session/cgroup.procs'"

echo "with-delegated-cgroup: $(cat /proc/self/cgroup)" >&2
exec "$@"
