package controller

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

func cleanupEvidenceDatabase(ctx context.Context, store service.TaskRequestEvidenceObjectStore, stats *evidenceRetentionStats, retentionSeconds int64, persistProgress func() error) error {
	var cursor int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		indexes, err := model.TaskRequestEvidenceDatabaseCandidates(stats.Cutoff, stats.TargetTimestamp, cursor, 100)
		if err != nil {
			return err
		}
		if len(indexes) == 0 {
			break
		}
		for _, index := range indexes {
			cursor = index.Id
			// Older releases may have marked bodies complete while leaving files.
			// Verify object deletion before losing their durable event-ID locator.
			if !processEvidenceRetentionItem(ctx, store, index, stats) {
				stats.FailedItems++
				continue
			}
			if err := purgeClosedEvidenceDatabase(ctx, index.Id, stats, persistProgress); err != nil {
				if err == model.ErrSystemTaskLockLost {
					return err
				}
				stats.DatabaseFailures++
				stats.FailedItems++
				stats.ErrorCategory = "database_cleanup_failed"
			}
		}
		if len(indexes) < 100 {
			break
		}
	}
	orphanCutoff := stats.Cutoff - retentionSeconds
	if stats.TargetTimestamp > 0 {
		orphanCutoff = stats.TargetTimestamp
	}
	var orphanCursor int64 = -1
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ids, err := model.OrphanTaskRequestEvidenceIDs(orphanCutoff, orphanCursor, 100)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			orphanCursor = id
			if !deleteEvidenceObjects(ctx, store, id, stats) {
				stats.FailedItems++
				continue
			}
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				deleted, err := model.DeleteOrphanTaskRequestEvidenceEventBatch(id, 500)
				if err != nil {
					return err
				}
				stats.DeletedEvents += deleted
				if err := persistProgress(); err != nil {
					return err
				}
				if deleted < 500 {
					break
				}
			}
		}
		if len(ids) < 100 {
			break
		}
	}
	// File-only namespaces have no index or event from which to discover them.
	// Their file timestamps supply the finite retention anchor.
	return store.WalkEvidenceNamespaces(ctx, func(ids []int64) error {
		existing, err := model.ExistingTaskRequestEvidenceIDs(ids)
		if err != nil {
			return err
		}
		live := make(map[int64]bool, len(existing))
		for _, id := range existing {
			live[id] = true
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return err
			}
			if live[id] {
				continue // Live indexes keep their creation/accepted-cleanup scope.
			}
			if err := store.WalkEvidenceFiles(ctx, id, func(file service.EvidenceStoredFile) error {
				if file.ModifiedAt >= orphanCutoff {
					return nil
				}
				size, err := store.Delete(file.Key)
				if err != nil {
					return err
				}
				if size > 0 {
					stats.DeletedFiles++
					stats.FreedBytes += size
				} else {
					stats.MissingFiles++
				}
				return nil
			}); err != nil {
				stats.FailedItems++
				stats.ErrorCategory = "delete_failed"
				continue
			}
			if id > 0 {
				if err := store.RemoveEmptyEvidenceDir(id); err != nil {
					var nonEmpty *service.EvidenceDirectoryNotEmptyError
					if !errors.As(err, &nonEmpty) {
						stats.FailedItems++
						stats.ErrorCategory = "directory_cleanup_failed"
					}
				}
			}
		}
		return persistProgress()
	})
}

// These lifetimes are independent of object availability and manual scope.
func cleanupEvidenceAuditAndHistory(ctx context.Context, stats *evidenceRetentionStats, persistProgress func() error) error {

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		deleted, err := model.DeleteExpiredEvidenceAccessBatch(stats.Cutoff, 500)
		if err != nil {
			return err
		}
		stats.DeletedAccessLogs += deleted
		if err := persistProgress(); err != nil {
			return err
		}
		if deleted < 500 {
			break
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		deleted, err := model.DeleteExpiredEvidenceTaskHistoryBatch(common.GetTimestamp(), 500)
		if err != nil {
			return err
		}
		stats.DeletedHistory += deleted
		if err := persistProgress(); err != nil {
			return err
		}
		if deleted < 500 {
			break
		}
	}
	return nil
}

func purgeClosedEvidenceDatabase(ctx context.Context, evidenceID int64, stats *evidenceRetentionStats, persistProgress func() error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		deleted, err := model.DeleteClosedEvidenceEventBatch(evidenceID, 500)
		if err != nil {
			return err
		}
		stats.DeletedEvents += deleted
		if err := persistProgress(); err != nil {
			return err
		}
		if deleted < 500 {
			break
		}
	}
	deleted, err := model.DeleteExpiredEvidenceIndex(evidenceID, stats.Cutoff)
	if err != nil {
		return err
	}
	stats.DeletedIndexes += deleted
	return persistProgress()
}
