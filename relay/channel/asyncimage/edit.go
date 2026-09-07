package asyncimage

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/service"
)

// Only byte inputs need staging; customer URLs stay untouched and ordered.
func (p *imagePayload) PrepareImageInputs(ctx context.Context) error {
	if len(p.inputs) == 0 {
		return nil
	}
	for _, input := range p.inputs {
		if input.IsURL() {
			continue
		}
		var err error
		ctx, err = service.WithImageObjectStore(ctx)
		if err == nil {
			err = service.CheckImageObjectStoreReady(ctx)
		}
		if err != nil {
			return errors.New("reference image upload requires available object storage")
		}
		break
	}
	urls := make([]string, len(p.inputs))
	for i, input := range p.inputs {
		if input.IsURL() {
			urls[i] = input.URL
			continue
		}
		key, err := service.BuildImageObjectKey("ephemeral", "input")
		if err != nil {
			return errors.New("reference image staging failed")
		}
		if _, err = service.PutImageObject(ctx, key, input.MimeType, input.Data); err != nil {
			return errors.New("reference image staging failed")
		}
		urls[i], err = service.PresignImageInputURL(ctx, key)
		if err != nil {
			return errors.New("reference image signing failed")
		}
	}
	p.ImageURLs = urls
	p.inputs = nil
	return nil
}
