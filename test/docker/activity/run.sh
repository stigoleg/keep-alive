#!/bin/sh
# Starts a mock logind and Xvfb, lets the X idle counter pass the threshold, runs
# keepalive --active for ~40 s while sampling xprintidle, then checks that
# the bursts reset the counter and that --json reported simulating/xdotool.
set -eu

# keepalive refuses to run without a sleep inhibitor. The container has no
# systemd, so a mocked logind on a private system bus provides one.
python3 /usr/local/bin/mock-logind /tmp/system-bus-address >/tmp/logind.log 2>&1 &
i=0
until [ -s /tmp/system-bus-address ]; do
	i=$((i + 1))
	[ "$i" -lt 100 ] || { echo "FAIL: mock logind did not start"; cat /tmp/logind.log; exit 1; }
	sleep 0.1
done
DBUS_SYSTEM_BUS_ADDRESS=$(cat /tmp/system-bus-address)
export DBUS_SYSTEM_BUS_ADDRESS

activity.test -test.run 'Test(XInput|Evdev|KernelCapability|XdotoolUser|Input|Playback|Monitor|Held|Default|Seed|Diagnostics)'

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

# Create an independent source before observation begins. Its XTEST devices
# have different identities from xdotool's default synthetic input.
xinput create-master keepalive-test-user

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

check "keepalive exited 0 after SIGTERM (got $status)" "$(if [ "$status" -eq 0 ]; then echo 0; else echo 1; fi)"

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
check "xprintidle dropped after bursts ($drops times, want >= 4)" "$(if [ "$drops" -ge 4 ]; then echo 0; else echo 1; fi)"

# After the first reset the counter must never climb past interval*1.35
# plus a burst and a tick.
peak=$(awk 'NR > 1 && $1 < prev - 1000 { armed = 1 } armed && $1 > max { max = $1 } { prev = $1 } END { print max + 0 }' /tmp/idle.log)
check "idle stayed below 10s once simulating (peak ${peak}ms)" "$(if [ "$peak" -lt 10000 ]; then echo 0; else echo 1; fi)"

now=$(xdotool getmouselocation --shell | grep -E '^[XY]=' | tr '\n' ' ')
check "pointer back at its origin ($origin-> $now)" "$(if [ "$origin" = "$now" ]; then echo 0; else echo 1; fi)"

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
# Start a fresh session for takeover. Interrupt in the middle of a real
# chained xdotool burst, then keep a key held beyond the idle threshold.
keepalive --plain --json --active --active-idle 10s --active-interval 5s -d 1m \
  >/tmp/takeover.ndjson 2>/tmp/takeover.err &
ka=$!
python3 /usr/local/bin/test-user-input key 12 /tmp/input-go &
user=$!
while [ ! -f /tmp/input-go.ready ]; do sleep 0.01; done
start=$(date +%s)
while :; do
  position=$(xdotool getmouselocation --shell | grep -E '^[XY]=' | tr '\n' ' ')
  [ "$position" != "$origin" ] && break
  [ "$(($(date +%s) - start))" -lt 20 ] || { echo "FAIL: no burst to interrupt"; cat /tmp/takeover.err; kill "$ka"; exit 1; }
  sleep 0.02
done
touch /tmp/input-go
sleep 0.2
stop_position=$(xdotool getmouselocation --shell | grep -E '^[XY]=' | tr '\n' ' ')
sleep 1
stable_position=$(xdotool getmouselocation --shell | grep -E '^[XY]=' | tr '\n' ' ')
[ "$stop_position" = "$stable_position" ] || { echo "FAIL: pointer kept moving after takeover"; kill "$ka"; exit 1; }
# A curved stroke can naturally cross the origin before the event arrives.
# Command-level tests separately assert that takeover issues no restore move.
wait "$user"
jq -e -s 'any(.[]; .type == "activity" and .snapshot.activity.state == "paused_user")' /tmp/takeover.ndjson >/dev/null
before=$(jq -r -s '[.[] | select(.snapshot.activity.last_burst != null) | .snapshot.activity.last_burst] | last' /tmp/takeover.ndjson)
sleep 5
after=$(jq -r -s '[.[] | select(.snapshot.activity.last_burst != null) | .snapshot.activity.last_burst] | last' /tmp/takeover.ndjson)
[ "$before" = "$after" ] || { echo "FAIL: did not wait for fresh idle after key release"; kill "$ka"; exit 1; }
# Motion, click and scroll from the separate source must each reset that wait.
for kind in motion button scroll; do
  python3 /usr/local/bin/test-user-input "$kind"
  sleep 1
done
kill -TERM "$ka"
wait "$ka"
echo "ok:   independent input cancelled chained playback without restoring the pointer"
echo "PASS"
