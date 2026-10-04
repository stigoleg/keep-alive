#!/bin/sh
# Applies the uinput udev rule and loads the module now, so --active works
# without a reboot. Never fails the install: containers and chroots have no
# udev or kernel modules to talk to.
if command -v udevadm >/dev/null 2>&1; then
	udevadm control --reload-rules >/dev/null 2>&1 || true
fi
if command -v modprobe >/dev/null 2>&1; then
	modprobe uinput >/dev/null 2>&1 || true
fi
if command -v udevadm >/dev/null 2>&1; then
	udevadm trigger --subsystem-match=misc --sysname-match=uinput >/dev/null 2>&1 || true
fi
exit 0
