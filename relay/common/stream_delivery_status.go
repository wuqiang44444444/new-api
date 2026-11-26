package common

import (
	"context"
	"errors"
)

func (o StreamOutcome) IsNormalEnd() bool {
	return o.EndReason.isNormal()
}

func (reason StreamEndReason) isNormal() bool {
	return reason == StreamEndReasonDone || reason == StreamEndReasonEOF || reason == StreamEndReasonHandlerStop
}

// MarkClientGoneOnDelivery records a failed final write after the scanner has
// joined. Keep an earlier abnormal transport outcome for diagnostics.
func (s *StreamStatus) MarkClientGoneOnDelivery(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.EndReason.isNormal() {
		s.EndReason = StreamEndReasonClientGone
		s.EndError = err
	}
}

// MarkCompletedAfterDelivery corrects a cancellation that won before a
// successfully delivered protocol completion could stop the scanner.
func (s *StreamStatus) MarkCompletedAfterDelivery() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.response == ResponseOutcomeCompleted && s.EndReason == StreamEndReasonClientGone &&
		errors.Is(s.EndError, context.Canceled) && s.ErrorCount == 0 {
		s.EndReason = StreamEndReasonDone
		s.EndError = nil
	}
}
