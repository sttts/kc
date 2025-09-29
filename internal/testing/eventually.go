package kctesting

import (
    "testing"
    "time"
)

// Eventually polls condition until it returns true or the timeout elapses.
// interval controls how often the condition is re-evaluated.
// On failure, t.Fatalf is called with the optional message.
func Eventually(t testing.TB, timeout, interval time.Duration, condition func() bool, msg ...string) {
    t.Helper()
    deadline := time.Now().Add(timeout)
    for {
        if condition() {
            return
        }
        if time.Now().After(deadline) {
            m := "condition not met within timeout"
            if len(msg) > 0 && msg[0] != "" { m = msg[0] }
            t.Fatalf(m)
        }
        time.Sleep(interval)
    }
}

