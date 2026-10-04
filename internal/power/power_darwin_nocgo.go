//go:build darwin && !cgo

package power

// Without cgo there is no IOKit; fall back to a caffeinate child.

func newPlatform() Inhibitor { return caffeinateInhibitor{} }

func platformMechanisms() []Mechanism { return []Mechanism{caffeinateMechanism()} }
