#!/bin/sh
# Drops the removed uinput rule from udev's cache. The module stays loaded
# until reboot; other software may be using it.
if command -v udevadm >/dev/null 2>&1; then
	udevadm control --reload-rules >/dev/null 2>&1 || true
fi
exit 0
