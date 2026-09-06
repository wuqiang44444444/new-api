package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/model"
)

// RecoverImageTaskArtifacts registers objects uploaded before a worker crashed.
// No Provider ID is required and no generation request is ever sent here.
// Missing objects remain unresolved: an object key cannot reconstruct lost bytes.
func RecoverImageTaskArtifacts(ctx context.Context, task *model.Task) (*ImageTaskExecution, error) {
	ctx, err := WithImageObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	data := task.PrivateData.ImageTask
	if data == nil || !data.GenerationComplete || len(data.ResultManifest) == 0 {
		return nil, nil
	}
	for _, artifact := range data.ResultManifest {
		exists, err := HeadImageObject(ctx, artifact.ObjectKey)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		won, err := model.AppendImageTaskArtifact(task, artifact)
		if err != nil {
			return nil, err
		}
		if !won {
			return nil, errors.New("image recovery lease lost")
		}
	}
	if len(task.PrivateData.ImageTask.Artifacts) != data.ExpectedImages {
		return nil, nil
	}
	return &ImageTaskExecution{Outcome: ImageTaskOutcomeSuccess, Images: task.PrivateData.ImageTask.Artifacts, Usage: data.Usage}, nil
}
