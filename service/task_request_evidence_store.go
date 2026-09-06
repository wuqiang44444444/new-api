package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// TaskRequestEvidenceObjectStore 是证据正文的私有存储合同。
// 一期提供本地加密目录实现；对象键与公开产物下载授权完全隔离，
// 不复用 TaskArtifactStore 的命名空间或访问策略。
type TaskRequestEvidenceObjectStore interface {
	Put(key string, plaintext []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
}

type localEncryptedEvidenceStore struct {
	baseDir string
	aead    cipher.AEAD
	timeout time.Duration
}

var (
	evidenceObjectStore   TaskRequestEvidenceObjectStore
	evidenceStoreInitOnce sync.Once
)

// GetTaskRequestEvidenceStore 惰性装配证据存储：首次调用发生在 InitEnv
// 之后，使用最终生效配置（含派生密钥）。
func GetTaskRequestEvidenceStore() TaskRequestEvidenceObjectStore {
	if !system_setting.GetTaskRequestEvidenceConfig().Enabled {
		return nil
	}
	evidenceStoreInitOnce.Do(func() {
		if err := InitTaskRequestEvidenceStore(system_setting.GetTaskRequestEvidenceConfig()); err != nil {
			common.SysError("task request evidence store init failed: " + err.Error())
		}
	})
	return evidenceObjectStore
}

// InitTaskRequestEvidenceStore 按配置装配证据存储；目录在首次写入时创建。
func InitTaskRequestEvidenceStore(config system_setting.TaskRequestEvidenceConfig) error {
	if err := system_setting.ValidateTaskRequestEvidenceConfig(config); err != nil {
		return err
	}
	if !config.Enabled {
		evidenceObjectStore = nil
		return nil
	}
	rawKey, err := hex.DecodeString(config.EncryptionKeyHex)
	if err != nil || len(rawKey) != 32 {
		return fmt.Errorf("evidence encryption key must be 32 bytes hex")
	}
	block, err := aes.NewCipher(rawKey)
	if err != nil {
		return fmt.Errorf("evidence cipher init failed: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("evidence gcm init failed: %w", err)
	}
	evidenceObjectStore = &localEncryptedEvidenceStore{
		baseDir: config.StorageDir,
		aead:    aead,
		timeout: time.Duration(config.WriteTimeoutSeconds) * time.Second,
	}
	return nil
}

func (s *localEncryptedEvidenceStore) Put(key string, plaintext []byte) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("evidence nonce failed: %w", err)
	}
	sealed := s.aead.Seal(nonce, nonce, plaintext, nil)
	timeout := s.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case evidenceWriteSlots <- struct{}{}:
	default:
		return fmt.Errorf("evidence store busy")
	}
	result := make(chan error, 1)
	go func() { defer func() { <-evidenceWriteSlots }(); result <- s.writeAtomically(key, sealed) }()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		return fmt.Errorf("evidence write timeout")
	}
}

func (s *localEncryptedEvidenceStore) writeAtomically(key string, payload []byte) error {
	path, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("evidence object dir failed: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".evidence-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if _, err := temp.Write(payload); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// Link commits without replacing an existing event object.
	if err := os.Link(temp.Name(), path); err != nil {
		return err
	}
	// Sync each parent so newly created directories survive a crash too.
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		dir, err := os.Open(parent)
		if err != nil {
			return err
		}
		syncErr := dir.Sync()
		closeErr := dir.Close()
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
		if parent == filepath.Dir(parent) || parent == "." {
			break
		}
	}

	return nil
}

func (s *localEncryptedEvidenceStore) Get(key string) ([]byte, error) {
	path, err := s.resolvePath(key)
	if err != nil {
		return nil, err
	}
	sealed, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrTaskRequestEvidenceUnavailable
		}
		return nil, fmt.Errorf("evidence object read failed: %w", err)
	}
	nonceSize := s.aead.NonceSize()
	if len(sealed) < nonceSize {
		return nil, errors.New("evidence object is truncated")
	}
	plaintext, err := s.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("evidence object decrypt failed: %w", err)
	}
	return plaintext, nil
}

func (s *localEncryptedEvidenceStore) Delete(key string) error {
	path, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("evidence object delete failed: %w", err)
	}
	return nil
}

func (s *localEncryptedEvidenceStore) resolvePath(key string) (string, error) {
	cleaned := strings.ReplaceAll(key, "\\", "/")
	for _, part := range strings.Split(cleaned, "/") {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("invalid evidence object key")
		}
	}
	return filepath.Join(s.baseDir, filepath.FromSlash(cleaned)), nil
}

// EvidenceSha256Hex 计算正文完整性校验值，写入事件记录。
func EvidenceSha256Hex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// Bound workers even when the filesystem stalls beyond the request deadline.
var evidenceWriteSlots = make(chan struct{}, 8)
