//go:build linux

package main

/*
#include <signal.h>

static void fix_signal_sa_onstack(int signum) {
	struct sigaction st;
	if (sigaction(signum, NULL, &st) < 0) return;
	if (!(st.sa_flags & SA_ONSTACK)) {
		st.sa_flags |= SA_ONSTACK;
		sigaction(signum, &st, NULL);
	}
}

static void fix_webkit_signal_handlers(void) {
	fix_signal_sa_onstack(SIGSEGV);
	fix_signal_sa_onstack(SIGBUS);
	fix_signal_sa_onstack(SIGFPE);
	fix_signal_sa_onstack(SIGILL);
	fix_signal_sa_onstack(SIGABRT);
}
*/
import "C"

import "time"

// WebKitGTK's JavaScriptCore installs SIGSEGV/SIGBUS handlers (used for GC
// thread suspension and Wasm trap handling) without SA_ONSTACK, which Go's
// runtime refuses to tolerate. Wails 2.12 runs install_signal_handlers() via
// g_idle_add after gtk_init, but JSC installs its handlers lazily when the
// first JavaScript context is created — usually after that idle pass. We
// re-apply SA_ONSTACK on a short interval so JSC's late install gets corrected
// regardless of when it lands.
func init() {
	go func() {
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			C.fix_webkit_signal_handlers()
		}
	}()
}
