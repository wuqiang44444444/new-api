package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Image error evidence is observation only. A bounded memory reservation covers
// both in-flight captures and queued writes. Read/Write only observe bytes.
// Legacy image errors queue their writes; all-request captures persist at
// handler completion. Evidence failures never change the business outcome.
var imageEvidenceSlots = make(chan struct{}, 8)
var imageEvidenceQueue = make(chan *ImageErrorEvidence, 8)
var imageEvidenceAccepted, imageEvidenceWritten, imageEvidenceFailed, imageEvidenceDropped atomic.Uint64

type imageEvidenceContextKey struct{}

type ImageErrorEvidence {
	Index             model.TaskRequestEvidence
	mu                sync.Mutex
	remaining         int
	int64
	records           []*imageEvidenceExchange
	responseRecords   map[*http.Response]*imageEvidenceExchange
	allRequests       bool
	deliveryStatsOnly bool
	finished          bool
}
