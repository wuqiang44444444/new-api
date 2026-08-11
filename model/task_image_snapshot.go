package model

func MediaImageTaskSnapshotIsCurrent(task *Task) bool {
	return task != nil && task.ClientProtocol == TaskClientProtocolOpenAIImages && task.PrivateData.MediaImage != nil
}
