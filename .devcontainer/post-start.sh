#!/bin/sh
set -eu

if ! docker info >/dev/null 2>&1; then
	sudo sh -c 'nohup dockerd --group docker --host=unix:///var/run/docker.sock >/tmp/dockerd.log 2>&1 &'
fi

i=0
until docker info >/dev/null 2>&1; do
	i=$((i + 1))
	if [ "$i" -ge 60 ]; then
		sudo tail -80 /tmp/dockerd.log
		exit 1
	fi
	sleep 1
done
