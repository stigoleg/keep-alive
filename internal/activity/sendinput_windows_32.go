//go:build windows && (386 || arm)

package activity

// wantInputSize is sizeof(INPUT) on 32-bit Windows.
const wantInputSize = 28
