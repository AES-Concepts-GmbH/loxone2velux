package klf200

import "time"

// SetLimitationFrameTimeout shortens the per-frame wait of the limitation
// status exchange for tests and returns a function that restores it.
func SetLimitationFrameTimeout(d time.Duration) (restore func()) {
	old := limitationFrameTimeout
	limitationFrameTimeout = d
	return func() { limitationFrameTimeout = old }
}
