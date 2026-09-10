package common

import "errors"

// ErrUpstreamObservationUnavailable marks a poll-time task-observation
// failure that is a local infrastructure condition — for example a plugin
// engine admission or execution timeout — rather than provider truth.
// Pollers that see this sentinel skip the round: the task keeps its state
// and billing, the failure is not counted toward the consecutive-failure
// cutoff, and no refund or task failure is manufactured. The existing
// overall deadline sweep remains the only backstop.
var ErrUpstreamObservationUnavailable = errors.New("upstream observation infrastructure unavailable")
