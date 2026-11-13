package model

import "sync"

// Prevent an option refresh started before a local commit from applying its
// older retention value after the new one is published to this process.
var taskRequestEvidenceOptionMutex sync.Mutex
