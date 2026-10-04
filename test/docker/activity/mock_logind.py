#!/usr/bin/env python3
"""Start a private system bus with python-dbusmock's logind template, so
keepalive can take its sleep inhibitor in a container without systemd.

Writes the bus address to the file given as the only argument once logind
answers, then runs until killed.
"""

import os
import signal
import subprocess
import sys

from dbusmock import DBusTestCase


def main() -> None:
    DBusTestCase.start_system_bus()
    DBusTestCase.spawn_server_template("logind", {}, stdout=subprocess.DEVNULL)
    with open(sys.argv[1] + ".tmp", "w") as f:
        f.write(os.environ["DBUS_SYSTEM_BUS_ADDRESS"])
    os.rename(sys.argv[1] + ".tmp", sys.argv[1])
    signal.pause()


if __name__ == "__main__":
    main()
