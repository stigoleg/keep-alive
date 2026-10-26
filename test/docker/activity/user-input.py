#!/usr/bin/python3
"""Send input through an independent XInput slave in the isolated Xvfb server.

The named master/slaves are created before keepalive starts. Their source IDs
are distinct from keepalive/xdotool's default XTEST devices, allowing the real
observer to distinguish test-user activity from simulated activity.
"""
from pathlib import Path
import ctypes as c
import subprocess
import sys
import time

x11 = c.CDLL("libX11.so.6")
xi = c.CDLL("libXi.so.6")
xtst = c.CDLL("libXtst.so.6")
x11.XOpenDisplay.argtypes = [c.c_char_p]
x11.XOpenDisplay.restype = c.c_void_p
x11.XSync.argtypes = [c.c_void_p, c.c_int]
x11.XCloseDisplay.argtypes = [c.c_void_p]
xi.XOpenDevice.argtypes = [c.c_void_p, c.c_ulong]
xi.XOpenDevice.restype = c.c_void_p
xi.XCloseDevice.argtypes = [c.c_void_p, c.c_void_p]
for name in ["XTestFakeDeviceKeyEvent", "XTestFakeDeviceButtonEvent"]:
    getattr(xtst, name).argtypes = [c.c_void_p, c.c_void_p, c.c_uint, c.c_int, c.POINTER(c.c_int), c.c_int, c.c_ulong]
xtst.XTestFakeDeviceMotionEvent.argtypes = [c.c_void_p, c.c_void_p, c.c_int, c.c_int, c.POINTER(c.c_int), c.c_int, c.c_ulong]

kind = sys.argv[1]
held = float(sys.argv[2]) if len(sys.argv) > 2 else 0.1
suffix = "keyboard" if kind == "key" else "pointer"
device_id = int(subprocess.check_output(["xinput", "list", "--id-only", f"keepalive-test-user XTEST {suffix}"]))
display = x11.XOpenDisplay(None)
if not display:
    raise SystemExit("cannot open test display")
device = xi.XOpenDevice(display, device_id)
if not device:
    raise SystemExit("cannot open independent test input device")
try:
    if len(sys.argv) > 3:
        trigger = Path(sys.argv[3])
        Path(str(trigger) + ".ready").touch()
        while not trigger.exists():
            time.sleep(0.001)
    if kind == "motion":
        axes = (c.c_int * 2)(5, 3)
        xtst.XTestFakeDeviceMotionEvent(display, device, 1, 0, axes, 2, 0)
        x11.XSync(display, 0)
    else:
        action = xtst.XTestFakeDeviceKeyEvent if kind == "key" else xtst.XTestFakeDeviceButtonEvent
        detail = 38 if kind == "key" else (4 if kind == "scroll" else 1)
        action(display, device, detail, 1, None, 0, 0)
        x11.XSync(display, 0)
        time.sleep(held)
        action(display, device, detail, 0, None, 0, 0)
        x11.XSync(display, 0)
finally:
    xi.XCloseDevice(display, device)
    x11.XCloseDisplay(display)
