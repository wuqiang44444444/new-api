package service

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/QuantumNous/new-api/clienterrlog"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	"github.com/abema/go-mp4"
	"github.com/tcolgate/mp3"
)

func storeHostedAudioVideo(ctx context.Context, req assetadapter.AssetRequest, objectKey, contentType string) (int64, error) {
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return 0, err
	}
	store, ok := session.store.(interface {
		putVideoResultObject(context.Context, string, string, io.Reader, int64) (int64, error)
	})
	if !ok {
		return 0, ErrTaskArtifactStoreDisabled
	}
	maxBytes := req.SourceMaxBytes
	if maxBytes <= 0 || maxBytes > funCloudMaterialMaxBytes {
		maxBytes = funCloudMaterialMaxBytes
	}

	// Validate the complete source before the object store can publish it.
	// Spooling bounds memory and lets MP4 metadata follow the media data.
	file, err := os.CreateTemp("", "hosted-media-*")
	if err != nil {
		return 0, errors.New("could not stage hosted media")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	size, err := io.Copy(file, io.LimitReader(req.Source, maxBytes+1))
	if size > maxBytes {
		clienterrlog.Attach(ctx, clienterrlog.Report{Stage: "content_validation", Reason: "source_too_large"})
		return 0, ErrInvalidAssetRequest
	}
	if err != nil {
		clienterrlog.Attach(ctx, clienterrlog.Report{Stage: "content_validation", Reason: "source_read_failed"})
		return 0, errors.New("could not read hosted media")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, errors.New("could not read staged hosted media")
	}

	var valid bool
	switch contentType {
	case "audio/mpeg":
		valid = validHostedMP3(ctx, bufio.NewReader(file))
	case "video/mp4":
		valid = validHostedMP4(ctx, file, uint64(size))
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !valid {
		clienterrlog.Attach(ctx, clienterrlog.Report{Stage: "content_validation", Reason: "invalid_media"})
		return 0, ErrInvalidAssetRequest
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, errors.New("could not read staged hosted media")
	}
	return store.putVideoResultObject(ctx, objectKey, contentType, file, maxBytes)
}

// A complete frame must begin at each audio boundary. The decoder's normal
// resynchronization would otherwise skip damaged bytes or a partial final frame.
func validHostedMP3(ctx context.Context, source *bufio.Reader) bool {
	if prefix, _ := source.Peek(3); string(prefix) == "ID3" {
		header := make([]byte, 10)
		if _, err := io.ReadFull(source, header); err != nil || header[3] < 2 || header[3] > 4 {
			return false
		}
		var size int64
		for _, b := range header[6:] {
			if b >= 128 {
				return false
			}
			size = size<<7 | int64(b)
		}
		if header[3] == 4 && header[5]&0x10 != 0 {
			size += 10 // ID3v2.4 footer
		}
		if _, err := io.CopyN(io.Discard, source, size); err != nil {
			return false
		}
	}
	decoder := mp3.NewDecoder(source)
	var frame mp3.Frame
	var hasFrame bool
	for ctx.Err() == nil {
		prefix, err := source.Peek(3)
		if err != nil {
			return errors.Is(err, io.EOF) && len(prefix) == 0 && hasFrame
		}
		if string(prefix) == "TAG" {
			if _, err := source.Discard(128); err != nil {
				return false
			}
			_, err := source.Peek(1)
			return hasFrame && errors.Is(err, io.EOF)
		}
		if prefix[0] != 0xff || prefix[1]&0xe0 != 0xe0 {
			return false
		}
		skipped := 0
		if decoder.Decode(&frame, &skipped) != nil || skipped != 0 || frame.Header().Layer() != mp3.Layer3 || frame.Duration() <= 0 {
			return false
		}
		hasFrame = true
	}
	return false
}

func validHostedMP4(ctx context.Context, source io.ReadSeeker, size uint64) bool {
	var prefix [512]byte
	n, _ := io.ReadFull(source, prefix[:])
	if http.DetectContentType(prefix[:n]) != "video/mp4" {
		return false
	}

	var hasMovie, hasMedia, pendingFragment bool
	for offset := uint64(0); offset < size; {
		if ctx.Err() != nil {
			return false
		}
		if _, err := source.Seek(int64(offset), io.SeekStart); err != nil {
			return false
		}
		box, err := mp4.ReadBoxInfo(source)
		if err != nil || box.Size < box.HeaderSize || box.Size > size-offset {
			return false
		}
		switch box.Type {
		case mp4.BoxTypeMoov():
			video, err := hostedMP4HasVideoTrack(ctx, source, box, 0)
			if err != nil || !video {
				return false
			}
			hasMovie = true
		case mp4.BoxTypeMoof():
			if pendingFragment {
				return false
			}
			pendingFragment = true
		case mp4.BoxTypeMdat():
			// An unbounded media box cannot prove that its payload was
			// complete when the source HTTP body ended.
			if box.ExtendToEOF || box.Size == box.HeaderSize {
				return false
			}
			hasMedia = true
			pendingFragment = false
		}
		offset += box.Size
	}
	return hasMovie && hasMedia && !pendingFragment
}

// Walk only the movie, track and media containers needed to identify a video
// track, without decoding or allocating sample tables from source metadata.
func hostedMP4HasVideoTrack(ctx context.Context, source io.ReadSeeker, parent *mp4.BoxInfo, depth int) (bool, error) {
	var video bool
	end := parent.Offset + parent.Size
	for offset := parent.Offset + parent.HeaderSize; offset < end; {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if _, err := source.Seek(int64(offset), io.SeekStart); err != nil {
			return false, err
		}
		box, err := mp4.ReadBoxInfo(source)
		if err != nil || box.ExtendToEOF || box.Size < box.HeaderSize || box.Size > end-offset {
			return false, ErrInvalidAssetRequest
		}
		switch {
		case depth == 0 && box.Type == mp4.BoxTypeTrak(), depth == 1 && box.Type == mp4.BoxTypeMdia():
			found, err := hostedMP4HasVideoTrack(ctx, source, box, depth+1)
			if err != nil {
				return false, err
			}
			video = video || found
		case depth == 2 && box.Type == mp4.BoxTypeHdlr():
			var header [12]byte // FullBox, predefined value, handler type
			if box.Size-box.HeaderSize < uint64(len(header)) {
				return false, ErrInvalidAssetRequest
			}
			if _, err := io.ReadFull(source, header[:]); err != nil {
				return false, err
			}
			video = video || string(header[8:]) == "vide"
		}
		offset += box.Size
	}
	return video, nil
}
