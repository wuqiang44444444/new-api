package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Automatic changes use the same database row lock as manual status/edit paths.
// Only committed before/after facts are audited; cache updates follow commit.
func updateAutomaticChannelStatus(channelID int, usingKey string, status int, reason string, audit ...ChannelStatusAudit) bool {
	if common.MemoryCacheEnabled {
		channelStatusLock.Lock()
		defer channelStatusLock.Unlock()
	}
	pollingLock := GetChannelPollingLock(channelID)
	pollingLock.Lock()
	defer pollingLock.Unlock()
	var before, after int
	written := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		channel, previous, didWrite, err := updateChannelStatusTx(tx, channelID, usingKey, status, reason, 0, true)
		before, written = previous, didWrite
		if err == nil {
			after = channel.Status
		}
		return err
	})
	if err != nil {
		common.SysError("failed to commit channel status change")
		return false
	}
	if written {
		recordChannelStatusTransition(channelID, before, after, ChannelStatusSourceAuto, 0, audit...)
		if common.MemoryCacheEnabled {
			InitChannelCache()
		}
	}
	return written
}
