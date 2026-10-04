#!/usr/bin/env python3
"""Run the Go D-Bus inhibitor test against python-dbusmock.

Starts a private system bus with dbusmock's logind template and a private
session bus with mocked org.freedesktop.ScreenSaver, org.gnome.SessionManager
and org.freedesktop.PowerManagement (the last one returns an int32 cookie,
like KDE's PowerDevil), then runs the internal/power tests with
KEEPALIVE_DBUSMOCK=1 so TestDBusMock runs too. It checks the mocks' call
logs and the logind lock list.
"""

import os
import subprocess
import sys

import dbus
import dbusmock
from dbusmock import DBusTestCase

SESSION_MOCKS = [
    ("org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver", [
        ("Inhibit", "ss", "u", "ret = 11"),
        ("UnInhibit", "u", "", ""),
    ]),
    ("org.gnome.SessionManager", "/org/gnome/SessionManager", "org.gnome.SessionManager", [
        ("Inhibit", "susu", "u", "ret = 22"),
        ("Uninhibit", "u", "", ""),
    ]),
    ("org.freedesktop.PowerManagement", "/org/freedesktop/PowerManagement/Inhibit",
     "org.freedesktop.PowerManagement.Inhibit", [
        ("Inhibit", "ss", "i", "ret = -33"),
        ("UnInhibit", "i", "", ""),
    ]),
]


def main() -> int:
    DBusTestCase.start_system_bus()
    DBusTestCase.start_session_bus()
    procs = []
    try:
        logind, _ = DBusTestCase.spawn_server_template("logind", {}, stdout=subprocess.DEVNULL)
        procs.append(logind)
        session = DBusTestCase.get_dbus(system_bus=False)
        for name, path, iface, methods in SESSION_MOCKS:
            procs.append(DBusTestCase.spawn_server(name, path, iface, system_bus=False, stdout=subprocess.DEVNULL))
            mock = dbus.Interface(session.get_object(name, path), dbusmock.MOCK_IFACE)
            for method, in_sig, out_sig, code in methods:
                mock.AddMethod(iface, method, in_sig, out_sig, code)

        env = dict(os.environ, KEEPALIVE_DBUSMOCK="1")
        cmd = ["go", "test", "-count=1", "-v", "./internal/power/"]
        return subprocess.call(cmd + sys.argv[1:], env=env)
    finally:
        for p in procs:
            p.terminate()
            p.wait()
        DBusTestCase.stop_dbus(DBusTestCase.system_bus_pid)
        DBusTestCase.stop_dbus(DBusTestCase.session_bus_pid)


if __name__ == "__main__":
    sys.exit(main())
