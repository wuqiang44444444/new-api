package model

// GetByOnlyTaskId loads a task by its public task ID without applying a user
// ownership filter. It is intended for internal lifecycle and polling paths.
func GetByOnlyTaskId(taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}

	var task *Task
	err := DB.Where("task_id = ?", taskId).First(&task).Error
	exists, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return task, exists, nil
}
