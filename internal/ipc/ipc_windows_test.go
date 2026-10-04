package ipc

import "os"

const helperEnv = "KEEPALIVE_IPC_TEST_SERVER"

func helperServer() { os.Exit(2) }
