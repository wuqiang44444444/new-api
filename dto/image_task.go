package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

const (
	ImageTaskObject           = kitdto.ImageTaskObject
	ImageTaskStatusQueued     = kitdto.ImageTaskStatusQueued
	ImageTaskStatusInProgress = kitdto.ImageTaskStatusInProgress
	ImageTaskStatusCompleted  = kitdto.ImageTaskStatusCompleted
	ImageTaskStatusFailed     = kitdto.ImageTaskStatusFailed
	ImageTaskStatusUnknown    = kitdto.ImageTaskStatusUnknown
)

type ImageTaskError = kitdto.ImageTaskError
type ImageTaskResult = kitdto.ImageTaskResult
type ImageTask = kitdto.ImageTask
