#!/bin/sh
# Starts Xvfb, lets the X idle counter pass the threshold, runs
# keepalive --active for ~40 s while sampling xprintidle, then checks that
# the bursts reset the counter and that --json reported simulating/xdotool.
set -eu

export DISPLAY=:99
# -noreset: without a long-lived client Xvfb regenerates (and zeroes the
# idle counter) every time the last client, e.g. xprintidle, disconnects.
Xvfb :99 -screen 0 1280x800x24 -nolisten tcp -noreset >/tmp/xvfb.log 2>&1 &
i=0
until xdotool getdisplaygeometry >/dev/null 2>&1; do
	i=$((i + 1))
	[ "$i" -lt 100 ] || { echo "FAIL: Xvfb did not start"; cat /tmp/xvfb.log; exit 1; }
	sleep 0.1
done

# Park the pointer away from the edges and remember where it is.
xdotool mousemove 640 400
origin=$(xdotool getmouselocation --shell | grep -E '^[XY]=' | tr '\n' ' ')

# --active-idle cannot go below 10s; start once the counter is past it so
# the first burst comes on the first tick.
echo "waiting for xprintidle > 11s"
deadline=$(($(date +%s) + 30))
while [ "$(xprintidle)" -lt 11000 ]; do
	[ "$(date +%s)" -lt "$deadline" ] || { echo "FAIL: X idle counter never passed 11s (now $(xprintidle)ms)"; exit 1; }
	sleep 0.5
done

keepalive --plain --json --active --active-idle 10s --active-interval 5s -d 1m \
	>/tmp/events.ndjson 2>/tmp/keepalive.err &
ka=$!

end=$(($(date +%s) + 40))
: >/tmp/idle.log
while [ "$(date +%s)" -lt "$end" ]; do
	echo "$(xprintidle)" >>/tmp/idle.log
	sleep 0.25
done
kill -TERM "$ka"
status=0
wait "$ka" || status=$?

fail=0
check() { # check <description> <condition-exit-status>
	if [ "$2" -eq 0 ]; then echo "ok:   $1"; else echo "FAIL: $1"; fail=1; fi
}

check "keepalive exited 0 after SIGTERM (got $status)" "$([ "$status" -eq 0 ]; echo $?)"

jq -e -s 'all(.[]; type == "object")' /tmp/events.ndjson >/dev/null
check "every output line is a JSON object" $?

jq -e -s 'any(.[]; .type == "activity" and .snapshot.activity.state == "simulating" and .snapshot.activity.method == "xdotool")' \
	/tmp/events.ndjson >/dev/null
check "--json reported state simulating with method xdotool" $?

jq -e -s 'all(.[]; .snapshot.activity.state != "paused_user" and .snapshot.activity.state != "degraded")' \
	/tmp/events.ndjson >/dev/null
check "never paused_user or degraded (our bursts were not mistaken for the user)" $?

# A drop of more than a second between samples is a burst resetting the
# counter. 5s ±35% over ~40s gives about seven.
drops=$(awk 'NR > 1 && $1 < prev - 1000 { n++ } { prev = $1 } END { print n + 0 }' /tmp/idle.log)
check "xprintidle dropped after bursts ($drops times, want >= 4)" "$([ "$drops" -ge 4 ]; echo $?)"

# After the first reset the counter must never climb past interval*1.35
# plus a burst and a tick.
peak=$(awk 'NR > 1 && $1 < prev - 1000 { armed = 1 } armed && $1 > max { max = $1 } { prev = $1 } END { print max + 0 }' /tmp/idle.log)
check "idle stayed below 10s once simulating (peak ${peak}ms)" "$([ "$peak" -lt 10000 ]; echo $?)"

now=$(xdotool getmouselocation --shell | grep -E '^[XY]=' | tr '\n' ' ')
check "pointer back at its origin ($origin-> $now)" "$([ "$origin" = "$now" ]; echo $?)"

echo "--- activity events"
jq -r 'select(.type == "activity") | [.time, .snapshot.activity.state, .snapshot.activity.method, .snapshot.activity.reason, (.snapshot.activity.last_burst // "-")] | @tsv' \
	/tmp/events.ndjson

if [ "$fail" -ne 0 ]; then
	echo "--- events"
	cat /tmp/events.ndjson
	echo "--- stderr"
	cat /tmp/keepalive.err
	echo "--- idle samples (ms)"
	tr '\n' ' ' </tmp/idle.log
	echo
	exit 1
fi
echo "PASS"
