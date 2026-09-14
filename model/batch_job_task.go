package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// createBatchJobForTaskTx participates in BOTH initial commit and shared attempt
// recovery. No accepted task may exist without its durable Batch execution facts.
func createBatchJobForTaskTx(tx *gorm.DB, attempt *TaskCreateAttempt, task *Task) error {
	if task.Platform != constant.TaskPlatformAzureBatch {
		return nil
	}
	var params CreateBatchJobParams
	if err := common.Unmarshal(attempt.BillingSnapshot, &params); err != nil {
		return err
	}
	if params.PublicId != task.TaskID || params.UserId != task.UserId || params.AppID != task.AppID || task.ClientProtocol != attempt.ClientProtocol {
		return errors.New("batch recovery identity does not match attempt")
	}
	params.TaskRowId = task.ID
	job, err := CreatePendingBatchJobTx(tx, params)
	if err != nil {
		return err
	}
	return AttachBatchJobUpstreamIdTx(tx, job.Id, task.PrivateData.UpstreamTaskID, "validating")
}

// GetBatchJobByTaskRowId loads the job row joined to a Task.
func GetBatchJobByTaskRowId(taskRowId int64) (*BatchJob, error) {
	if taskRowId <= 0 {
		return nil, ErrBatchJobNotFound
	}
	var job BatchJob
	if err := DB.Where("task_row_id = ?", taskRowId).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBatchJobNotFound
		}
		return nil, err
	}
	return &job, nil
}

// SelectEnabledBatchChannel returns the deterministic enabled Batch channel
// for one group and public model: the lowest id wins. Batch routing is
// typed and explicit; it never uses priority, weight or random dispatch.
func SelectEnabledBatchChannel(group string, publicModel string) (*Channel, error) {
	if group == "" || publicModel == "" {
		return nil, errors.New("batch channel selection requires group and model")
	}
	var channels []Channel
	query := ApplyChannelGroupFilter(DB.Model(&Channel{}), group).
		Where("type = ? AND status = ?", constant.ChannelTypeAzureBatch, common.ChannelStatusEnabled).
		Order("id ASC")
	if err := query.Find(&channels).Error; err != nil {
		return nil, err
	}
	for i := range channels {
		for _, candidate := range channels[i].GetModels() {
			if candidate == publicModel {
				return &channels[i], nil
			}
		}
	}
	return nil, errors.New("no enabled batch channel serves this model in the group")
}
