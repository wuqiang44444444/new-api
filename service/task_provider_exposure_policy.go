package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/provider_exposure_setting"
)

const (
	providerExposurePolicyVersion = "v1"
	providerExposurePolicyLimit   = 100
)

type providerExposurePolicyMetrics struct {
	count            int64
	quota            int64
	ratePerHour      float64
	conversionRatio  float64
	oldestAgeSeconds int64
	oldestCreatedAt  int64
}

type providerExposureThreshold struct {
	count           int64
	quota           int64
	ratePerHour     float64
	conversionRatio float64
	oldestAge       int64
}

func EvaluateProviderExposurePolicies(ctx context.Context, limit int) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > providerExposurePolicyLimit {
		limit = providerExposurePolicyLimit
	}
	if err := model.EnsureProviderExposureIncidents(limit); err != nil {
		logger.LogWarn(ctx, "create provider exposure incidents failed: "+err.Error())
		return 0
	}
	now := time.Now().Unix()
	incidents, err := model.GetProviderExposureIncidentsDue(now, limit)
	if err != nil {
		logger.LogWarn(ctx, "query provider exposure incidents failed: "+err.Error())
		return 0
	}
	processed := 0
	for i := range incidents {
		if ctx.Err() != nil {
			break
		}
		incident := &incidents[i]
		claimed, claimErr := model.ClaimProviderExposureIncident(incident.ID, now)
		if claimErr != nil {
			logger.LogWarn(ctx, fmt.Sprintf("claim provider exposure incident %d failed: %v", incident.ID, claimErr))
			continue
		}
		if !claimed {
			continue
		}
		if err := evaluateProviderExposureIncident(ctx, incident, now); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("evaluate provider exposure incident %d failed: %v", incident.ID, err))
			continue
		}
		processed++
	}
	return processed
}

func evaluateProviderExposureIncident(ctx context.Context, incident *model.ProviderExposureIncident, now int64) error {
	if incident == nil {
		return fmt.Errorf("provider exposure incident is required")
	}
	setting := provider_exposure_setting.Current()
	profile, err := providerExposureProfile(incident)
	if err != nil {
		return err
	}
	if !setting.ActiveForImplementation(incident.LinkImplementationID, incident.LinkImplementationVer) {
		_, err := model.CompleteProviderExposureIncidentEvaluation(
			incident.ID,
			model.ProviderExposureIncidentEvaluation{
				Status:          model.ProviderExposureIncidentIgnored,
				Severity:        model.ProviderExposureSeverityNone,
				Action:          model.ProviderExposureActionNone,
				UpstreamProfile: profile,
				PolicyVersion:   providerExposurePolicyVersion,
			},
		)
		return err
	}

	metrics, err := providerExposureMetricsForIncident(incident, profile, setting.WindowSeconds, now)
	if err != nil {
		return err
	}
	warning := providerExposureThreshold{
		count:           setting.WarningCount,
		quota:           setting.WarningQuota,
		ratePerHour:     setting.WarningRatePerHour,
		conversionRatio: setting.WarningConversionRatio,
		oldestAge:       setting.WarningOldestAgeSeconds,
	}
	paging := providerExposureThreshold{
		count:           setting.PagingCount,
		quota:           setting.PagingQuota,
		ratePerHour:     setting.PagingRatePerHour,
		conversionRatio: setting.PagingConversionRatio,
		oldestAge:       setting.PagingOldestAgeSeconds,
	}
	autoDisable := providerExposureThreshold{
		count:           setting.AutoDisableCount,
		quota:           setting.AutoDisableQuota,
		ratePerHour:     setting.AutoDisableRatePerHour,
		conversionRatio: setting.AutoDisableConversionRatio,
		oldestAge:       setting.AutoDisableOldestAgeSeconds,
	}

	severity := model.ProviderExposureSeverityNone
	status := model.ProviderExposureIncidentObserved
	if providerExposureThresholdReached(metrics, warning) {
		severity = model.ProviderExposureSeverityWarning
		status = model.ProviderExposureIncidentOpen
	}
	if providerExposureThresholdReached(metrics, paging) {
		severity = model.ProviderExposureSeverityPaging
		status = model.ProviderExposureIncidentOpen
	}

	action := model.ProviderExposureActionNone
	remaining, err := model.CountEnabledEquivalentVideoCandidates(incident.PublicModel)
	if err != nil {
		return err
	}
	if setting.AutoDisablePublicModelEnabled && providerExposureThresholdReached(metrics, autoDisable) {
		action = model.ProviderExposureActionModelDisabled
		if _, err := model.DisableProviderExposurePublicModel(incident.ChannelID, incident.PublicModel); err != nil {
			return err
		}
		model.InitChannelCache()
		remaining, err = model.CountEnabledEquivalentVideoCandidates(incident.PublicModel)
		if err != nil {
			return err
		}
		severity = model.ProviderExposureSeverityPaging
		status = model.ProviderExposureIncidentOpen
	}

	notify := severity == model.ProviderExposureSeverityPaging &&
		(incident.Severity != model.ProviderExposureSeverityPaging || incident.NotificationSentAt == 0)
	notificationAt := incident.NotificationSentAt
	if notify {
		notificationAt = now
	}
	nextEvaluationAt := nextProviderExposureAgeEvaluation(now, metrics.oldestCreatedAt, setting, action)
	completed, err := model.CompleteProviderExposureIncidentEvaluation(
		incident.ID,
		model.ProviderExposureIncidentEvaluation{
			Status:                        status,
			Severity:                      severity,
			Action:                        action,
			UpstreamProfile:               profile,
			ExposureCount:                 metrics.count,
			CustomerQuotaReleased:         metrics.quota,
			ExposureRatePerHour:           metrics.ratePerHour,
			UnknownToExposureRatio:        metrics.conversionRatio,
			OldestExposureAgeSeconds:      metrics.oldestAgeSeconds,
			RemainingEquivalentCandidates: remaining,
			PolicyVersion:                 providerExposurePolicyVersion,
			NextEvaluationAt:              nextEvaluationAt,
			NotificationSentAt:            notificationAt,
		},
	)
	if err != nil || !completed {
		if err == nil {
			err = fmt.Errorf("provider exposure incident evaluation lost its state")
		}
		return err
	}

	if severity != model.ProviderExposureSeverityNone {
		logger.LogWarn(ctx, fmt.Sprintf(
			"provider exposure policy triggered: incident=%d reason=%s severity=%s action=%s channel_id=%d public_model=%s profile=%s count=%d released_quota=%d rate_per_hour=%.4f unknown_conversion=%.4f oldest_age_seconds=%d remaining_candidates=%d",
			incident.ID,
			incident.Reason,
			severity,
			action,
			incident.ChannelID,
			incident.PublicModel,
			profile,
			metrics.count,
			metrics.quota,
			metrics.ratePerHour,
			metrics.conversionRatio,
			metrics.oldestAgeSeconds,
			remaining,
		))
	}
	if notify {
		subject := fmt.Sprintf("Provider exposure paging: %s", incident.PublicModel)
		content := fmt.Sprintf(
			"Incident #%d (%s) affected public SKU %s on channel #%d (profile %s). "+
				"Exposure count: %d; released quota: %d; remaining equivalent candidates: %d; action: %s.",
			incident.ID,
			incident.Reason,
			incident.PublicModel,
			incident.ChannelID,
			profile,
			metrics.count,
			metrics.quota,
			remaining,
			action,
		)
		NotifyRootUser("provider_exposure_paging", subject, content)
	}
	return nil
}

func providerExposureProfile(incident *model.ProviderExposureIncident) (string, error) {
	if profile := strings.TrimSpace(incident.UpstreamProfile); profile != "" {
		return profile, nil
	}
	return model.ResolveProviderExposureProfile(incident.ExposureID)
}

func providerExposureMetricsForIncident(
	incident *model.ProviderExposureIncident,
	profile string,
	windowSeconds, now int64,
) (providerExposurePolicyMetrics, error) {
	since := now - windowSeconds
	aggregates, err := model.ProviderExposureAggregates(
		since,
		incident.ChannelID,
		incident.PublicModel,
		profile,
		incident.LinkImplementationID,
		incident.LinkImplementationVer,
	)
	if err != nil {
		return providerExposurePolicyMetrics{}, err
	}
	metrics := providerExposurePolicyMetrics{}
	for _, aggregate := range aggregates {
		metrics.count += aggregate.ExposureCount
		metrics.quota += aggregate.CustomerQuotaReleased
		if metrics.oldestCreatedAt == 0 || aggregate.OldestCreatedAt < metrics.oldestCreatedAt {
			metrics.oldestCreatedAt = aggregate.OldestCreatedAt
		}
	}
	if windowSeconds > 0 {
		metrics.ratePerHour = float64(metrics.count) * 3600 / float64(windowSeconds)
	}
	if metrics.oldestCreatedAt > 0 && now > metrics.oldestCreatedAt {
		metrics.oldestAgeSeconds = now - metrics.oldestCreatedAt
	}
	unknownAttempts, releasedAttempts, err := model.CountTaskCreateUnknownOutcomes(
		since,
		incident.ChannelID,
		incident.PublicModel,
		profile,
		incident.LinkImplementationID,
		incident.LinkImplementationVer,
	)
	if err != nil {
		return providerExposurePolicyMetrics{}, err
	}
	if unknownAttempts > 0 {
		metrics.conversionRatio = float64(releasedAttempts) / float64(unknownAttempts)
	}
	return metrics, nil
}

func providerExposureThresholdReached(metrics providerExposurePolicyMetrics, threshold providerExposureThreshold) bool {
	return (threshold.count > 0 && metrics.count >= threshold.count) ||
		(threshold.quota > 0 && metrics.quota >= threshold.quota) ||
		(threshold.ratePerHour > 0 && metrics.ratePerHour >= threshold.ratePerHour) ||
		(threshold.conversionRatio > 0 && metrics.conversionRatio >= threshold.conversionRatio) ||
		(threshold.oldestAge > 0 && metrics.oldestAgeSeconds >= threshold.oldestAge)
}

func nextProviderExposureAgeEvaluation(
	now, oldestCreatedAt int64,
	setting provider_exposure_setting.PolicySetting,
	action string,
) int64 {
	if action == model.ProviderExposureActionModelDisabled || oldestCreatedAt <= 0 {
		return 0
	}
	next := int64(0)
	for _, age := range []int64{
		setting.WarningOldestAgeSeconds,
		setting.PagingOldestAgeSeconds,
		setting.AutoDisableOldestAgeSeconds,
	} {
		if age <= 0 {
			continue
		}
		at := oldestCreatedAt + age
		if at <= now {
			continue
		}
		if next == 0 || at < next {
			next = at
		}
	}
	return next
}

type ProviderExposureMetric struct {
	ChannelID                int     `json:"channel_id"`
	PublicModel              string  `json:"public_model"`
	UpstreamProfile          string  `json:"upstream_profile"`
	LinkImplementationID     string  `json:"link_implementation_id"`
	LinkImplementationVer    string  `json:"link_implementation_version"`
	Reason                   string  `json:"reason"`
	ExposureCount            int64   `json:"exposure_count"`
	CustomerQuotaReleased    int64   `json:"customer_quota_released"`
	ExposureRatePerHour      float64 `json:"exposure_rate_per_hour"`
	UnknownToExposureRatio   float64 `json:"unknown_to_exposure_ratio"`
	OldestExposureAgeSeconds int64   `json:"oldest_exposure_age_seconds"`
	OldestCreatedAt          int64   `json:"oldest_created_at"`
	NewestCreatedAt          int64   `json:"newest_created_at"`
}

type ProviderExposureMetricsSummary struct {
	WindowSeconds            int64                                   `json:"window_seconds"`
	GeneratedAt              int64                                   `json:"generated_at"`
	ExposureCount            int64                                   `json:"exposure_count"`
	CustomerQuotaReleased    int64                                   `json:"customer_quota_released"`
	ExposureRatePerHour      float64                                 `json:"exposure_rate_per_hour"`
	OldestExposureAgeSeconds int64                                   `json:"oldest_exposure_age_seconds"`
	Metrics                  []ProviderExposureMetric                `json:"metrics"`
	Policy                   provider_exposure_setting.PolicySetting `json:"policy"`
}

func GetProviderExposureMetrics(windowSeconds int64) (*ProviderExposureMetricsSummary, error) {
	if windowSeconds < 60 {
		windowSeconds = provider_exposure_setting.Current().WindowSeconds
	}
	if windowSeconds > 30*24*60*60 {
		windowSeconds = 30 * 24 * 60 * 60
	}
	now := time.Now().Unix()
	aggregates, err := model.ProviderExposureAggregates(now-windowSeconds, 0, "", "")
	if err != nil {
		return nil, err
	}
	summary := &ProviderExposureMetricsSummary{
		WindowSeconds: windowSeconds,
		GeneratedAt:   now,
		Metrics:       make([]ProviderExposureMetric, 0, len(aggregates)),
		Policy:        provider_exposure_setting.Current(),
	}
	oldestCreatedAt := int64(0)
	for _, aggregate := range aggregates {
		unknownAttempts, releasedAttempts, countErr := model.CountTaskCreateUnknownOutcomes(
			now-windowSeconds,
			aggregate.ChannelID,
			aggregate.PublicModel,
			aggregate.UpstreamProfile,
			aggregate.LinkImplementationID,
			aggregate.LinkImplementationVer,
		)
		if countErr != nil {
			return nil, countErr
		}
		conversion := float64(0)
		if unknownAttempts > 0 && aggregate.Reason == string(model.TaskCreateAttemptReleasedWithExposure) {
			conversion = float64(releasedAttempts) / float64(unknownAttempts)
		}
		age := int64(0)
		if aggregate.OldestCreatedAt > 0 && now > aggregate.OldestCreatedAt {
			age = now - aggregate.OldestCreatedAt
		}
		summary.Metrics = append(summary.Metrics, ProviderExposureMetric{
			ChannelID:                aggregate.ChannelID,
			PublicModel:              aggregate.PublicModel,
			UpstreamProfile:          aggregate.UpstreamProfile,
			LinkImplementationID:     aggregate.LinkImplementationID,
			LinkImplementationVer:    aggregate.LinkImplementationVer,
			Reason:                   aggregate.Reason,
			ExposureCount:            aggregate.ExposureCount,
			CustomerQuotaReleased:    aggregate.CustomerQuotaReleased,
			ExposureRatePerHour:      float64(aggregate.ExposureCount) * 3600 / float64(windowSeconds),
			UnknownToExposureRatio:   conversion,
			OldestExposureAgeSeconds: age,
			OldestCreatedAt:          aggregate.OldestCreatedAt,
			NewestCreatedAt:          aggregate.NewestCreatedAt,
		})
		summary.ExposureCount += aggregate.ExposureCount
		summary.CustomerQuotaReleased += aggregate.CustomerQuotaReleased
		if oldestCreatedAt == 0 || aggregate.OldestCreatedAt < oldestCreatedAt {
			oldestCreatedAt = aggregate.OldestCreatedAt
		}
	}
	summary.ExposureRatePerHour = float64(summary.ExposureCount) * 3600 / float64(windowSeconds)
	if oldestCreatedAt > 0 && now > oldestCreatedAt {
		summary.OldestExposureAgeSeconds = now - oldestCreatedAt
	}
	return summary, nil
}

func ResolveProviderExposureIncident(
	incidentID int64,
	operatorID int,
	note string,
	restorePublicModel bool,
) (*model.ProviderExposureResolutionResult, error) {
	result, err := model.ResolveProviderExposureIncident(
		incidentID,
		operatorID,
		note,
		restorePublicModel,
	)
	if err != nil {
		return nil, err
	}
	if result.Restored {
		model.InitChannelCache()
	}
	return result, nil
}
