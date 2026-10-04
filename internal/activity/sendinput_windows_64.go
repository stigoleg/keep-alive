//go:build windows && (amd64 || arm64)

package activity

// wantInputSize is sizeof(INPUT) on 64-bit Windows.
const wantInputSize = 40
