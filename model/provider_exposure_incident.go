package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProviderExposureIncidentStatus string

const (
	ProviderExposureIncidentPending    ProviderExposureIncidentStatus = "pending"
	ProviderExposureIncidentEvaluating ProviderExposureIncidentStatus = "evaluating"
	ProviderExposureIncidentOpen       ProviderExposureIncidentStatus = "open"
	ProviderExposureIncidentObserved   ProviderExposureIncidentStatus = "observed"
	ProviderExposureIncidentIgnored    ProviderExposureIncidentStatus = "ignored"
	ProviderExposureIncidentResolved   ProviderExposureIncidentStatus = "resolved"

	ProviderExposureSeverityNone    = "none"
	ProviderExposureSeverityWarning = "warning"
	ProviderExposureSeverityPaging  = "paging"

	ProviderExposureActionNone          = "none"
	ProviderExposureActionModelDisabled = "public_model_disabled"
)

// ProviderExposureIncident is the durable policy/audit side of an immutable
// ProviderCostExposure. The unique exposure_id makes paging and circuit-breaker
// actions idempotent across scheduler retries.
type ProviderExposureIncident struct {
	ID                            int64                          `json:"id" gorm:"primaryKey"`
	ExposureID                    int64                          `json:"exposure_id" gorm:"uniqueIndex"`
	Status                        ProviderExposureIncidentStatus `json:"status" gorm:"type:varchar(20);index"`
	Severity                      string                         `json:"severity" gorm:"type:varchar(16);index"`
	Action                        string                         `json:"action" gorm:"type:varchar(32);index"`
	Reason                        string                         `json:"reason" gorm:"type:varchar(64);index"`
	ChannelID                     int                            `json:"channel_id" gorm:"index"`
	PublicModel                   string                         `json:"public_model" gorm:"type:varchar(191);index"`
	UpstreamProfile               string                         `json:"upstream_profile" gorm:"type:varchar(64);index"`
	LinkImplementationID          string                         `json:"link_implementation_id" gorm:"type:varchar(128);index"`
	LinkImplementationVer         string                         `json:"link_implementation_version" gorm:"column:link_implementation_version;type:varchar(32);index"`
	LinkImplementationHash        string                         `json:"link_implementation_hash" gorm:"type:varchar(80)"`
	ExposureCount                 int64                          `json:"exposure_count"`
	CustomerQuotaReleased         int64                          `json:"customer_quota_released"`
	ExposureRatePerHour           float64                        `json:"exposure_rate_per_hour"`
	UnknownToExposureRatio        float64                        `json:"unknown_to_exposure_ratio"`
	OldestExposureAgeSeconds      int64                          `json:"oldest_exposure_age_seconds"`
	RemainingEquivalentCandidates int                            `json:"remaining_equivalent_candidates"`
	PolicyVersion                 string                         `json:"policy_version" gorm:"type:varchar(32)"`
	NextEvaluationAt              int64                          `json:"next_evaluation_at" gorm:"bigint;index"`
	NotificationSentAt            int64                          `json:"notification_sent_at" gorm:"bigint"`
	CreatedAt                     int64                          `json:"created_at" gorm:"bigint;index"`
	UpdatedAt                     int64                          `json:"updated_at" gorm:"bigint"`
	ResolvedAt                    int64                          `json:"resolved_at" gorm:"bigint;index"`
	ResolvedBy                    int                            `json:"resolved_by"`
	ResolutionNote                string                         `json:"resolution_note,omitempty" gorm:"type:text"`
}

type ProviderExposureAggregate struct {
	ChannelID             int    `json:"channel_id"`
	PublicModel           string `json:"public_model"`
	UpstreamProfile       string `json:"upstream_profile"`
	LinkImplementationID  string `json:"link_implementation_id"`
	LinkImplementationVer string `json:"link_implementation_version" gorm:"column:link_implementation_version"`
	Reason                string `json:"reason"`
	ExposureCount         int64  `json:"exposure_count"`
	CustomerQuotaReleased int64  `json:"customer_quota_released"`
	OldestCreatedAt       int64  `json:"oldest_created_at"`
	NewestCreatedAt       int64  `json:"newest_created_at"`
}

func EnsureProviderExposureIncidents(limit int) error {
	if limit <= 0 {
		return nil
	}
	var exposures []ProviderCostExposure
	err := DB.Where("NOT EXISTS (?)",
		DB.Model(&ProviderExposureIncident{}).
			Select("1").
			Where("provider_exposure_incidents.exposure_id = provider_cost_exposures.id"),
	).Order("id").Limit(limit).Find(&exposures).Error
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	for i := range exposures {
		exposure := &exposures[i]
		incident := &ProviderExposureIncident{
			ExposureID:             exposure.ID,
			Status:                 ProviderExposureIncidentPending,
			Severity:               ProviderExposureSeverityNone,
			Action:                 ProviderExposureActionNone,
			Reason:                 exposure.Reason,
			ChannelID:              exposure.ChannelID,
			PublicModel:            exposure.PublicModel,
			UpstreamProfile:        exposure.UpstreamProfile,
			LinkImplementationID:   exposure.LinkImplementationID,
			LinkImplementationVer:  exposure.LinkImplementationVer,
			LinkImplementationHash: exposure.LinkImplementationHash,
			NextEvaluationAt:       now,
			CreatedAt:              now,
			UpdatedAt:              now,
		}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(incident).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetProviderExposureIncidentsDue(now int64, limit int) ([]ProviderExposureIncident, error) {
	if limit <= 0 {
		return nil, nil
	}
	var incidents []ProviderExposureIncident
	err := DB.Where(
		"(status = ?) OR (status IN ? AND next_evaluation_at > ? AND next_evaluation_at <= ?)",
		ProviderExposureIncidentPending,
		[]ProviderExposureIncidentStatus{
			ProviderExposureIncidentEvaluating,
			ProviderExposureIncidentOpen,
			ProviderExposureIncidentObserved,
		},
		0,
		now,
	).Order("next_evaluation_at, id").Limit(limit).Find(&incidents).Error
	return incidents, err
}

func ClaimProviderExposureIncident(id, now int64) (bool, error) {
	if id <= 0 {
		return false, errors.New("provider exposure incident is required")
	}
	result := DB.Model(&ProviderExposureIncident{}).
		Where("id = ? AND ((status = ?) OR (status IN ? AND next_evaluation_at > ? AND next_evaluation_at <= ?))",
			id,
			ProviderExposureIncidentPending,
			[]ProviderExposureIncidentStatus{
				ProviderExposureIncidentEvaluating,
				ProviderExposureIncidentOpen,
				ProviderExposureIncidentObserved,
			},
			0,
			now,
		).
		Updates(map[string]any{
			"status":             ProviderExposureIncidentEvaluating,
			"next_evaluation_at": now + 60,
			"updated_at":         now,
		})
	return result.RowsAffected == 1, result.Error
}

type ProviderExposureIncidentEvaluation struct {
	Status                        ProviderExposureIncidentStatus
	Severity                      string
	Action                        string
	UpstreamProfile               string
	ExposureCount                 int64
	CustomerQuotaReleased         int64
	ExposureRatePerHour           float64
	UnknownToExposureRatio        float64
	OldestExposureAgeSeconds      int64
	RemainingEquivalentCandidates int
	PolicyVersion                 string
	NextEvaluationAt              int64
	NotificationSentAt            int64
}

func CompleteProviderExposureIncidentEvaluation(id int64, evaluation ProviderExposureIncidentEvaluation) (bool, error) {
	if id <= 0 || evaluation.Status == "" || strings.TrimSpace(evaluation.Severity) == "" ||
		strings.TrimSpace(evaluation.Action) == "" {
		return false, errors.New("provider exposure incident evaluation is incomplete")
	}
	now := common.GetTimestamp()
	result := DB.Model(&ProviderExposureIncident{}).
		Where("id = ? AND status = ?", id, ProviderExposureIncidentEvaluating).
		Updates(map[string]any{
			"status":                          evaluation.Status,
			"severity":                        evaluation.Severity,
			"action":                          evaluation.Action,
			"upstream_profile":                evaluation.UpstreamProfile,
			"exposure_count":                  evaluation.ExposureCount,
			"customer_quota_released":         evaluation.CustomerQuotaReleased,
			"exposure_rate_per_hour":          evaluation.ExposureRatePerHour,
			"unknown_to_exposure_ratio":       evaluation.UnknownToExposureRatio,
			"oldest_exposure_age_seconds":     evaluation.OldestExposureAgeSeconds,
			"remaining_equivalent_candidates": evaluation.RemainingEquivalentCandidates,
			"policy_version":                  evaluation.PolicyVersion,
			"next_evaluation_at":              evaluation.NextEvaluationAt,
			"notification_sent_at":            evaluation.NotificationSentAt,
			"updated_at":                      now,
		})
	return result.RowsAffected == 1, result.Error
}

func ProviderExposureAggregates(since int64, channelID int, publicModel, profile string, implementationIdentity ...string) ([]ProviderExposureAggregate, error) {
	query := DB.Model(&ProviderCostExposure{}).
		Select(
			"channel_id, public_model, upstream_profile, link_implementation_id, link_implementation_version, reason, "+
				"COUNT(*) AS exposure_count, "+
				"COALESCE(SUM(customer_quota_released), 0) AS customer_quota_released, "+
				"MIN(created_at) AS oldest_created_at, MAX(created_at) AS newest_created_at",
		).
		Where("created_at >= ?", since)
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if publicModel = strings.TrimSpace(publicModel); publicModel != "" {
		query = query.Where("public_model = ?", publicModel)
	}
	if profile = strings.TrimSpace(profile); profile != "" {
		query = query.Where("upstream_profile = ?", profile)
	}
	if len(implementationIdentity) >= 2 {
		query = query.Where("link_implementation_id = ? AND link_implementation_version = ?", strings.TrimSpace(implementationIdentity[0]), strings.TrimSpace(implementationIdentity[1]))
	}
	var aggregates []ProviderExposureAggregate
	err := query.Group("channel_id, public_model, upstream_profile, link_implementation_id, link_implementation_version, reason").
		Order("channel_id, public_model, link_implementation_id, link_implementation_version, upstream_profile, reason").
		Scan(&aggregates).Error
	return aggregates, err
}

func CountTaskCreateUnknownOutcomes(
	since int64,
	channelID int,
	publicModel, profile string,
	implementationIdentity ...string,
) (int64, int64, error) {
	query := DB.Model(&TaskCreateAttempt{}).
		Where("outcome_unknown_at >= ? AND channel_id = ? AND public_model = ? AND upstream_profile = ?",
			since, channelID, strings.TrimSpace(publicModel), strings.TrimSpace(profile))
	if len(implementationIdentity) >= 2 {
		query = query.Where("link_implementation_id = ? AND link_implementation_version = ?", strings.TrimSpace(implementationIdentity[0]), strings.TrimSpace(implementationIdentity[1]))
	}
	var unknownCount int64
	if err := query.Count(&unknownCount).Error; err != nil {
		return 0, 0, err
	}
	var releasedCount int64
	if err := query.Where("status = ?", TaskCreateAttemptReleasedWithExposure).
		Count(&releasedCount).Error; err != nil {
		return 0, 0, err
	}
	return unknownCount, releasedCount, nil
}

func ResolveProviderExposureProfile(exposureID int64) (string, error) {
	var exposure ProviderCostExposure
	if err := DB.First(&exposure, "id = ?", exposureID).Error; err != nil {
		return "", err
	}
	if profile := strings.TrimSpace(exposure.UpstreamProfile); profile != "" {
		return profile, nil
	}
	switch exposure.SourceKind {
	case ProviderCostExposureSourceAttempt:
		var attempt TaskCreateAttempt
		if err := DB.Select("upstream_profile").
			First(&attempt, "attempt_id = ?", exposure.SourceID).Error; err != nil {
			return "", err
		}
		return strings.TrimSpace(attempt.UpstreamProfile), nil
	case ProviderCostExposureSourceTask:
		var task Task
		if err := DB.Select("private_data").First(&task, "task_id = ?", exposure.SourceID).Error; err != nil {
			return "", err
		}
		return strings.TrimSpace(string(task.PrivateData.VideoUpstreamProfile)), nil
	default:
		return "", nil
	}
}

func CountEnabledEquivalentVideoCandidates(publicModel string) (int, error) {
	publicModel = strings.TrimSpace(publicModel)
	if publicModel == "" {
		return 0, nil
	}
	var channelIDs []int
	err := DB.Model(&Ability{}).
		Where("model = ? AND enabled = ?", publicModel, true).
		Distinct("channel_id").
		Pluck("channel_id", &channelIDs).Error
	if err != nil {
		return 0, err
	}
	count := 0
	for _, channelID := range channelIDs {
		var channel Channel
		if err := DB.First(&channel, "id = ? AND status = ?", channelID, common.ChannelStatusEnabled).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return 0, err
		}
		capability, registered := ResolveVideoSKUCapability(publicModel)
		if !registered {
			continue
		}
		if ValidateVideoSKUImplementation(capability, &channel) != nil {
			continue
		}
		count++
	}
	return count, nil
}

func DisableProviderExposurePublicModel(channelID int, publicModel string) (bool, error) {
	result := DB.Model(&Ability{}).
		Where("channel_id = ? AND model = ? AND enabled = ?", channelID, strings.TrimSpace(publicModel), true).
		Update("enabled", false)
	return result.RowsAffected > 0, result.Error
}

func HasProviderExposurePolicyWork(now int64) bool {
	var id int64
	err := DB.Model(&ProviderCostExposure{}).
		Where("NOT EXISTS (?)",
			DB.Model(&ProviderExposureIncident{}).
				Select("1").
				Where("provider_exposure_incidents.exposure_id = provider_cost_exposures.id"),
		).
		Limit(1).
		Pluck("id", &id).Error
	if err == nil && id != 0 {
		return true
	}
	id = 0
	err = DB.Model(&ProviderExposureIncident{}).
		Where(
			"(status = ?) OR (status IN ? AND next_evaluation_at > ? AND next_evaluation_at <= ?)",
			ProviderExposureIncidentPending,
			[]ProviderExposureIncidentStatus{
				ProviderExposureIncidentEvaluating,
				ProviderExposureIncidentOpen,
				ProviderExposureIncidentObserved,
			},
			0,
			now,
		).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func ListProviderExposureIncidents(status string, limit, offset int) ([]ProviderExposureIncident, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	query := DB.Model(&ProviderExposureIncident{})
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var incidents []ProviderExposureIncident
	err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&incidents).Error
	return incidents, err
}

type ProviderExposureResolutionResult struct {
	Incident *ProviderExposureIncident
	Restored bool
}

func ResolveProviderExposureIncident(id int64, operatorID int, note string, restore bool) (*ProviderExposureResolutionResult, error) {
	validatedNote, noteErr := validateOperationalAuditNote(note)
	if id <= 0 || operatorID <= 0 || noteErr != nil {
		return nil, errors.New("provider exposure resolution is incomplete")
	}
	note = validatedNote
	result := &ProviderExposureResolutionResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var incident ProviderExposureIncident
		if err := lockForUpdate(tx).First(&incident, "id = ?", id).Error; err != nil {
			return err
		}
		if incident.Status == ProviderExposureIncidentResolved {
			result.Incident = &incident
			return nil
		}
		if incident.Status != ProviderExposureIncidentOpen && incident.Status != ProviderExposureIncidentObserved {
			return errors.New("provider exposure incident is not resolvable")
		}
		if restore {
			if incident.Action != ProviderExposureActionModelDisabled {
				return errors.New("provider exposure incident did not disable a public model")
			}
			var otherOpen int64
			if err := tx.Model(&ProviderExposureIncident{}).
				Where("id != ? AND channel_id = ? AND public_model = ? AND status = ? AND action = ?",
					incident.ID,
					incident.ChannelID,
					incident.PublicModel,
					ProviderExposureIncidentOpen,
					ProviderExposureActionModelDisabled,
				).Count(&otherOpen).Error; err != nil {
				return err
			}
			if otherOpen != 0 {
				return errors.New("another unresolved exposure still blocks this public model")
			}
			var channel Channel
			if err := lockForUpdate(tx).First(&channel, "id = ?", incident.ChannelID).Error; err != nil {
				return err
			}
			if channel.Status != common.ChannelStatusEnabled {
				return errors.New("channel is not enabled; public model cannot be restored")
			}
			capability, registered := ResolveVideoSKUCapability(incident.PublicModel)
			if !registered {
				return errors.New("public video model no longer has a published capability")
			}
			if err := ValidateVideoSKUImplementation(capability, &channel); err != nil {
				return err
			}
			update := tx.Model(&Ability{}).
				Where("channel_id = ? AND model = ? AND enabled = ?",
					incident.ChannelID, incident.PublicModel, false).
				Update("enabled", true)
			if update.Error != nil {
				return update.Error
			}
			result.Restored = update.RowsAffected > 0
		}
		now := common.GetTimestamp()
		update := tx.Model(&ProviderExposureIncident{}).
			Where("id = ? AND status IN ?", incident.ID, []ProviderExposureIncidentStatus{
				ProviderExposureIncidentOpen,
				ProviderExposureIncidentObserved,
			}).
			Updates(map[string]any{
				"status":             ProviderExposureIncidentResolved,
				"next_evaluation_at": 0,
				"resolved_at":        now,
				"resolved_by":        operatorID,
				"resolution_note":    note,
				"updated_at":         now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("provider exposure incident resolution lost its state")
		}
		incident.Status = ProviderExposureIncidentResolved
		incident.NextEvaluationAt = 0
		incident.ResolvedAt = now
		incident.ResolvedBy = operatorID
		incident.ResolutionNote = note
		incident.UpdatedAt = now
		result.Incident = &incident
		return nil
	})
	return result, err
}
