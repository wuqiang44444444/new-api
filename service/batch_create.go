package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	azurebatch "github.com/QuantumNous/new-api/relay/channel/azurebatch"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// Batch create is synchronous through upstream acceptance: validate → freeze
// pricing → durable attempt → fund hold → upload file → create batch →
// atomic Task + job commit. The north batch id is returned only after the
// upstream accepted the job.

var (
	ErrBatchJobModelDenied = errors.New("model is not available for batch processing")
	ErrBatchJobRejected    = errors.New("batch job was rejected before acceptance")
)

type BatchCreateResult struct {
	Job *model.BatchJob
}

func CreateBatchJob(c *gin.Context, request *dto.BatchCreateRequest) (*BatchCreateResult, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	userId := c.GetInt("id")
	tokenId := c.GetInt("token_id")

	file, err := model.GetBatchFileOwned(request.InputFileId, userId, tokenId)
	if err != nil {
		if errors.Is(err, model.ErrBatchFileNotFound) {
			return nil, &dto.BatchValidateError{Message: "input_file_id does not exist or is not owned by the caller"}
		}
		return nil, err
	}
	if file.Purpose != "batch" {
		return nil, &dto.BatchValidateError{Message: "input_file_id must reference a batch input file"}
	}
	publicModel := file.ModelName
	if c.GetBool("token_model_limit_enabled") {
		allowed, _ := c.Get("token_model_limit")
		limits, ok := allowed.(map[string]bool)
		if !ok || (!limits[publicModel] && !limits[ratio_setting.FormatMatchingModelName(publicModel)]) {
			return nil, ErrBatchJobModelDenied
		}
	}
	if strings.TrimSpace(publicModel) == "" {
		return nil, &dto.BatchValidateError{Message: "the stored batch file has no resolvable model"}
	}

	// Contract keys keep their authorization boundary: the bound rule must
	// authorize this model and must resolve to a Batch channel, otherwise the
	// request is rejected before any hold or upload.
	fact, err := resolveBatchContractFact(c, userId, publicModel)
	if err != nil {
		return nil, err
	}
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if fact != nil {
		group = fact.RouteGroup
	}

	pin := 0
	if fact != nil {
		pin = fact.ChannelId
	}
	channel, err := model.SelectEnabledBatchChannel(group, publicModel, pin)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBatchJobModelDenied, err)
	}
	if fact != nil && fact.ChannelId > 0 && fact.ChannelId != channel.Id {
		return nil, fmt.Errorf("%w: the bound contract rule does not resolve to the batch channel", ErrBatchJobModelDenied)
	}
	mapping := map[string]string{}
	if raw := channel.GetModelMapping(); raw != "" {
		if err := common.UnmarshalJsonStr(raw, &mapping); err != nil {
			return nil, errors.New("batch model mapping is invalid")
		}
	}
	deployment, _, err := model.ResolveModelMapping(publicModel, mapping)
	if err != nil {
		return nil, err
	}

	// Full re-validation against the stored content: an upload-time pass or
	// summary never substitutes for the create-time check.
	data, err := readBatchObjectForCreate(c.Request.Context(), file)
	if err != nil {
		return nil, err
	}
	converted, err := ConvertBatchJSONLForDeployment(data, deployment)
	if err != nil {
		return nil, err
	}

	if converted.Parse.Model != publicModel || converted.Parse.Checksum != file.Checksum {
		return nil, errors.New("stored batch input differs from its validated file facts")
	}

	expr, err := LoadBatchBillingForModel(publicModel)
	if err != nil {
		return nil, err
	}
	lineParams, err := freezeBatchPricingParameters(expr, data)
	if err != nil {
		return nil, err
	}
	connection := model.BatchConnection{BaseURL: channel.GetBaseURL(), Key: channel.Key, APIVersion: channel.Other}
	frozen := model.BatchFrozenSnapshot{
		AdapterVersion: azurebatch.AdapterVersion, LineParams: lineParams,
		Connection:       connection,
		Expr:             expr,
		ExprHash:         billingexpr.ExprHashString(expr),
		PricingTime:      time.Now().UTC().Unix(),
		QuotaPerUnit:     common.QuotaPerUnit,
		GroupRatio:       batchGroupRatio(userGroup, group),
		PublicModel:      publicModel,
		Deployment:       deployment,
		ChannelId:        channel.Id,
		Endpoint:         request.Endpoint,
		CompletionWindow: request.CompletionWindow,
		ContractFact:     fact,
		LineInputs:       converted.LineInputs,
	}
	estimate, err := estimateBatchJobQuota(&frozen, converted.LineEstimates)
	if err != nil {
		return nil, err
	}
	frozen.EstimateQuota = estimate

	publicJobId, err := model.NewBatchJobPublicId()
	if err != nil {
		return nil, err
	}
	metadata := ""
	if len(request.Metadata) != 0 {
		encoded, err := common.Marshal(request.Metadata)
		if err != nil {
			return nil, err
		}
		metadata = string(encoded)
	}
	params := model.CreateBatchJobParams{
		PublicId: publicJobId, UserId: userId, AppID: tokenId, TokenId: tokenId,
		ChannelId: channel.Id, PublicModel: publicModel, Deployment: deployment,
		InputFileId: file.Id, Endpoint: request.Endpoint, CompletionWindow: request.CompletionWindow,
		Metadata: metadata, Frozen: frozen, EstimateQuota: estimate, LineCount: converted.Parse.LineCount,
	}
	billingSnapshot, err := common.Marshal(params)
	if err != nil {
		return nil, err
	}
	connectionSnapshot, err := common.Marshal(connection)
	if err != nil {
		return nil, err
	}
	// Durable attempt first; funds and sending commit atomically afterwards.
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:             publicJobId,
		UserID:                   userId,
		TokenID:                  tokenId,
		AppID:                    tokenId,
		ClientProtocol:           "azurebatch",
		RequestHash:              file.Checksum,
		ChannelID:                channel.Id,
		PublicModel:              publicModel,
		UpstreamProfile:          "azure_batch",
		UpstreamProtocol:         azurebatch.AdapterVersion,
		AdapterVersion:           azurebatch.AdapterVersion,
		FrozenConnectionSnapshot: connectionSnapshot,
		BillingSnapshot:          billingSnapshot,
		HeldQuota:                estimate,
	})
	if err != nil {
		return nil, err
	}
	userSetting, _ := common.GetContextKeyType[kitdto.UserSetting](c, constant.ContextKeyUserSetting)
	hold, err := holdTaskAttemptForBilling(&relaycommon.RelayInfo{UserId: userId, TokenId: tokenId, OriginModelName: publicModel, UserSetting: userSetting, PriceData: hosttypes.PriceData{Quota: estimate}}, attempt.ID)
	if err != nil {
		_, _ = model.ReleaseTaskCreateAttemptHold(attempt.ID, model.TaskCreateAttemptRejected)
		return nil, fmt.Errorf("failed to reserve quota for the batch job: %w", err)
	}

	task := &model.Task{
		TaskID:         publicJobId,
		ClientProtocol: "azurebatch",
		Platform:       constant.TaskPlatformAzureBatch,
		UserId:         userId,
		AppID:          tokenId,
		Group:          group,
		ChannelId:      channel.Id,
		Quota:          estimate,
		Action:         "batch",
		Status:         model.TaskStatusInProgress,
		SubmitTime:     common.GetTimestamp(),
		StartTime:      common.GetTimestamp(),
		Properties:     model.Properties{OriginModelName: publicModel, UpstreamModelName: deployment},
	}
	task.PrivateData = model.TaskPrivateData{
		BillingContext: &model.TaskBillingContext{OriginModelName: publicModel, GroupRatio: frozen.GroupRatio, ContractFact: fact},
		AsyncBilling:   &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		BillingSource:  hold.BillingSource,
		SubscriptionId: hold.SubscriptionID,
		TokenId:        tokenId,
	}
	if err := model.RecordTaskCreateAttemptRecoveryTemplate(attempt.ID, task); err != nil {
		_, _ = model.ReleaseTaskCreateAttemptHold(attempt.ID, model.TaskCreateAttemptRejected)
		return nil, err
	}
	client := azurebatch.NewClient(channel.GetBaseURL(), channel.Key, channel.Other)
	uploadId, err := client.UploadBatchFile(c.Request.Context(), file.Id+".jsonl", converted.Bytes)
	if err != nil {
		_, _ = model.ReleaseTaskCreateAttemptHold(attempt.ID, model.TaskCreateAttemptRejected)
		return nil, fmt.Errorf("%w: upstream file upload failed", ErrBatchJobRejected)
	}
	status, err := client.CreateBatch(c.Request.Context(), uploadId, request.CompletionWindow)
	if err != nil {
		if errors.Is(err, azurebatch.ErrUpstreamRejected) {
			_, _ = model.ReleaseTaskCreateAttemptHold(attempt.ID, model.TaskCreateAttemptRejected)
			return nil, fmt.Errorf("%w: upstream refused the batch job", ErrBatchJobRejected)
		}
		// The request may or may not have been accepted: keep the durable
		// attempt and the hold for manual reconciliation. Never auto-retry.
		if markErr := model.MarkTaskCreateAttemptUnknown(attempt.ID, ""); markErr != nil {
			common.SysError("batch create attempt unknown mark failed: " + markErr.Error())
		}
		return nil, fmt.Errorf("batch job acceptance could not be confirmed; the request is held for reconciliation")
	}
	if status == nil || strings.TrimSpace(status.Id) == "" {
		_ = model.MarkTaskCreateAttemptUnknown(attempt.ID, "")
		return nil, errors.New("batch acceptance could not be confirmed; held for reconciliation")
	}
	task.PrivateData.UpstreamTaskID = status.Id
	if err := model.RecordTaskCreateAttemptUpstreamSuccess(attempt.ID, task); err != nil {
		return nil, err
	}
	if err := model.InsertTaskWithCreateAttempt(task, 0, attempt.ID); err != nil {
		return nil, err
	}
	job, err := model.GetBatchJobByTaskRowId(task.ID)
	if err != nil {
		return nil, err
	}
	return &BatchCreateResult{Job: job}, nil
}

// resolveBatchContractFact returns the frozen contract fact for a batch
// request. Unbound keys and disabled contracts return (nil, nil); load
// failures and model denials are hard errors — never native fallbacks.
func resolveBatchContractFact(c *gin.Context, userId int, publicModel string) (*hosttypes.ContractBillingFact, error) {
	contractId, _ := common.GetContextKeyType[int](c, constant.ContextKeyTokenContractId)
	if contractId <= 0 {
		return nil, nil
	}
	authVersion, ok := common.GetContextKeyType[int64](c, constant.ContextKeyAuthVersion)
	if !ok || authVersion <= 0 {
		return nil, fmt.Errorf("%w: authorization version is unavailable", ErrCustomerContractUnavailable)
	}
	fact, err := ResolveContractEntityRule(userId, authVersion, contractId, publicModel)
	if err != nil {
		return nil, err
	}
	return fact, nil
}

func batchGroupRatio(userGroup string, routeGroup string) float64 {
	ratio, _ := ResolveCustomerContractNativeGroupRatio(userGroup, routeGroup)
	return ratio
}

func readBatchObjectForCreate(ctx context.Context, file *model.BatchFile) ([]byte, error) {
	if file.ObjectKey == "" {
		return nil, ErrBatchObjectNotFound
	}
	exists, err := BatchObjectExists(ctx, file.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("check batch file content: %w", err)
	}
	if !exists {
		return nil, ErrBatchObjectNotFound
	}
	buf := batchInputBuffer{}
	if _, err := GetBatchObject(ctx, file.ObjectKey, &buf); err != nil {
		return nil, fmt.Errorf("read batch file content: %w", err)
	}
	return buf.buffer.Bytes(), nil
}

// A replaced or corrupt object must not bypass the upload memory bound.
type batchInputBuffer struct{ buffer bytes.Buffer }

func (b *batchInputBuffer) Write(p []byte) (int, error) {
	if len(p) > dto.MaxBatchFileBytes-b.buffer.Len() {
		return 0, errors.New("batch input exceeds the supported file size")
	}
	return b.buffer.Write(p)
}
