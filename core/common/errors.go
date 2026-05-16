// Package common provides shared primitives across olcrtc-core.
package common

import "errors"

// Sentinel errors used throughout the core.
var (
	ErrClosed         = errors.New("component is closed")
	ErrNotStarted     = errors.New("component is not started")
	ErrAlreadyStarted = errors.New("component is already started")
	ErrTimeout        = errors.New("operation timed out")
	ErrNotFound       = errors.New("not found")
	ErrInvalidConfig  = errors.New("invalid configuration")
	ErrAuthFailed     = errors.New("authentication failed")
	ErrCapacityExceed = errors.New("capacity exceeded")
)