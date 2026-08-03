package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// refundTaskAttempt handles the durable-attempt refund path and reports
// whether Refund must stop instead of entering the legacy asynchronous path.
func (s *BillingSession) refundTaskAttempt() bool {
	s.mu.Lock()
	if s.taskAttemptID == 0 {
		s.mu.Unlock()
		return false
	}
	if s.settled || s.refunded {
		s.mu.Unlock()
		return true
	}
	attemptID := s.taskAttemptID
	s.mu.Unlock()

	if _, err := model.ReleaseTaskCreateAttemptHold(attemptID, model.TaskCreateAttemptRejected); err != nil {
		common.SysLog("error releasing durable task attempt hold: " + err.Error())
		return true
	}
	s.mu.Lock()
	s.refunded = true
	s.mu.Unlock()
	return true
}
