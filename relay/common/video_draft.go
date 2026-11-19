package common

func (info *TaskRelayInfo) IsVideoDraftOperation() bool {
	return info != nil && (info.VideoDraftRequested || info.VideoDraftSourceID != "")
}
