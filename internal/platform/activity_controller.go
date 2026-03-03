package platform

import (
	"log"
	"sync/atomic"
	"time"
)

// IdleDetector returns the current system idle time.
type IdleDetector func() (time.Duration, error)

// JitterExecutor executes a mouse jitter pattern.
type JitterExecutor func(points []MousePoint, sessionDuration time.Duration)

// ActivityController encapsulates the shared idle-detection, jitter-gating, and
// logging logic used by all platforms for chat-app activity simulation. Each
// platform provides an IdleDetector and JitterExecutor; the controller handles
// the common state machine (synthetic idle tracking, interval enforcement, etc.).
type ActivityController struct {
	// platformName is used for log prefixes (e.g. "darwin", "windows", "linux").
	platformName string

	// patternGen generates mouse movement patterns.
	patternGen *MousePatternGenerator

	// lastActiveLogNS: last time we logged that user is active (unix nanos).
	lastActiveLogNS int64
	// lastJitterNS: last time we executed activity jitter (unix nanos).
	lastJitterNS int64
	// lastUserActiveNS: last time user activity was observed (unix nanos).
	lastUserActiveNS int64
}

// NewActivityController creates a new ActivityController.
func NewActivityController(platformName string, patternGen *MousePatternGenerator) *ActivityController {
	return &ActivityController{
		platformName: platformName,
		patternGen:   patternGen,
		lastUserActiveNS: time.Now().UnixNano(),
	}
}

// Reset clears all timing state. Call on Stop().
func (ac *ActivityController) Reset() {
	atomic.StoreInt64(&ac.lastActiveLogNS, 0)
	atomic.StoreInt64(&ac.lastJitterNS, 0)
	atomic.StoreInt64(&ac.lastUserActiveNS, 0)
}

// MaybeJitter checks idle state and, if conditions are met, executes a jitter
// pattern via the provided executor. Returns true if a jitter was performed.
func (ac *ActivityController) MaybeJitter(getIdle IdleDetector, execute JitterExecutor) bool {
	idle, err := getIdle()

	nowNS := time.Now().UnixNano()
	lastActiveLog := atomic.LoadInt64(&ac.lastActiveLogNS)
	lastJitterNS := atomic.LoadInt64(&ac.lastJitterNS)
	lastUserActiveNS := atomic.LoadInt64(&ac.lastUserActiveNS)

	if err != nil {
		if lastActiveLog == 0 || time.Duration(nowNS-lastActiveLog) > 2*time.Minute {
			atomic.StoreInt64(&ac.lastActiveLogNS, nowNS)
			log.Printf("%s: idle detection failed (%v); skipping activity simulation to avoid interference", ac.platformName, err)
		}
		return false
	}

	// Detect real user activity since our last synthetic jitter.
	// If observed idle time is significantly less than expected, user moved the mouse.
	if lastJitterNS != 0 {
		expectedIdle := time.Duration(nowNS - lastJitterNS)
		if expectedIdle > 0 && idle+SyntheticIdleResetTolerance < expectedIdle {
			atomic.StoreInt64(&ac.lastJitterNS, 0)
			atomic.StoreInt64(&ac.lastUserActiveNS, observedActiveTimestamp(nowNS, idle))
			if lastActiveLog == 0 || time.Duration(nowNS-lastActiveLog) > 2*time.Minute {
				atomic.StoreInt64(&ac.lastActiveLogNS, nowNS)
				log.Printf("%s: user activity detected (idle: %v); pausing activity simulation", ac.platformName, idle)
			}
			return false
		}
	}

	// Check if user is idle enough to simulate activity.
	idleQualified := idle >= IdleThreshold || lastJitterNS != 0
	if !idleQualified {
		atomic.StoreInt64(&ac.lastJitterNS, 0)
		atomic.StoreInt64(&ac.lastUserActiveNS, observedActiveTimestamp(nowNS, idle))
		if lastActiveLog == 0 || time.Duration(nowNS-lastActiveLog) > 2*time.Minute {
			atomic.StoreInt64(&ac.lastActiveLogNS, nowNS)
			log.Printf("%s: user is active (idle: %v); skipping simulation to avoid interference", ac.platformName, idle)
		}
		return false
	}

	if lastUserActiveNS != 0 && time.Duration(nowNS-lastUserActiveNS) < IdleThreshold {
		return false
	}

	// User became idle — log transition.
	if lastActiveLog != 0 {
		atomic.StoreInt64(&ac.lastActiveLogNS, 0)
		log.Printf("%s: user became idle (%v); resuming activity simulation", ac.platformName, idle)
	}

	// Enforce minimum interval between jitter sessions.
	if lastJitterNS != 0 && time.Duration(nowNS-lastJitterNS) < ChatAppActivityInterval {
		return false
	}

	// Execute jitter.
	points := ac.patternGen.GenerateRoundJitterPoints()
	sessionDuration := ac.patternGen.JitterSessionDuration()
	execute(points, sessionDuration)
	atomic.StoreInt64(&ac.lastJitterNS, nowNS)

	log.Printf("%s: idle detected (%v); jittered round mouse pattern (%v)", ac.platformName, idle, sessionDuration)
	return true
}
