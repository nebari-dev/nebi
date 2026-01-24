package main

import "os"

// osExit is the exit function. Tests override this to intercept exits.
var osExit = os.Exit
