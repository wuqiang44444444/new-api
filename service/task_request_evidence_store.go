package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// TaskRequestEvidenceObjectStore 是证据正文的私有存储合同。
// 一期提供本地加密目录实现；对象键与公开产物下载授权完全隔离，
// 不复用 TaskArtifactStore 的命名空间或访问策略。Delete 返回实际
// 回收的字节数（对象不存在返回 0 与 nil，视为幂等成功）。
type TaskRequestEvidenceObjectStore interface {
	Put(key string, plaintext []byte) error
	PutEventBody(evidenceID, eventID int64, plaintext []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) (int64, error)
	// RemoveEmptyEvidenceDir 在对象全部回收后清理该证据的安全空目录；
	// 目录不存在视为成功；非空目录报告未归属对象，不猜测删除。
	RemoveEmptyEvidenceDir(evidenceId int64) error
	// Inventory is confined to the private evidence root. Namespace zero means
	// loose root files; positive IDs identify one non-recursive namespace.
	EvidenceNamespaces() ([]int64, error)
	WalkEvidenceFiles(context.Context, int64, func(EvidenceStoredFile) error) error
}

type localEncryptedEvidenceStore struct {
	baseDir string
	aead    cipher.AEAD
	timeout time.Duration
}

var (
	evidenceObjectStore        TaskRequestEvidenceObjectStore
	evidenceStoreInitOnce      sync.Once
	ErrEvidenceDecryptFailed   = errors.New("evidence object could not be decrypted or authenticated")
	ErrEvidenceIntegrityFailed = errors.New("evidence object integrity check failed")
)

type EvidenceDirectoryNotEmptyError struct{ Files int }

func (e *EvidenceDirectoryNotEmptyError) Error() string {
	return "evidence directory contains unaccounted files"
}

// GetTaskRequestEvidenceStore 惰性装配证据存储：首次调用发生在 InitEnv
// 之后，使用最终生效配置（含派生密钥）。存储读取与删除和采集开关解耦：
// 关闭采集只停止新增正文，不暂停历史读取与过期清理。
func GetTaskRequestEvidenceStore() TaskRequestEvidenceObjectStore {
	evidenceStoreInitOnce.Do(func() {
		if err := InitTaskRequestEvidenceStore(system_setting.GetTaskRequestEvidenceConfig()); err != nil {
			common.SysError("task request evidence store init failed: " + err.Error())
		}
	})
	return evidenceObjectStore
}

// InitTaskRequestEvidenceStore 按配置装配证据存储；目录在首次写入时创建，
// 装配本身不创建任何目录，无历史对象时清理也不会产生对象目录。重复调用
// 先复位为不可用，再按目录与密钥装配：读取与清理不依赖采集开关。
func InitTaskRequestEvidenceStore(config system_setting.TaskRequestEvidenceConfig) error {
	evidenceObjectStore = nil
	if strings.TrimSpace(config.StorageDir) == "" {
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
	return s.put(key, plaintext, 0)
}

func (s *localEncryptedEvidenceStore) PutEventBody(evidenceID, eventID int64, plaintext []byte) error {
	return s.put(EvidenceObjectKey(evidenceID, eventID), plaintext, evidenceID)
}

func (s *localEncryptedEvidenceStore) put(key string, plaintext []byte, evidenceID int64) error {
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
	case <-timer.C:
		return ErrTaskRequestEvidenceStoreBusy
	}
	result := make(chan error, 1)
	go func() { defer func() { <-evidenceWriteSlots }(); result <- s.writeAtomically(key, sealed, evidenceID) }()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		return ErrTaskRequestEvidenceWriteTimeout
	}
}

func (s *localEncryptedEvidenceStore) writeAtomically(key string, payload []byte, evidenceID int64) (writeErr error) {
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
	defer func() {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		if writeErr != nil && evidenceID > 0 {
			_ = s.RemoveEmptyEvidenceDir(evidenceID)
		}
	}()
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
	publish := func() error { return os.Link(temp.Name(), path) }
	if evidenceID > 0 {
		err = model.CommitTaskRequestEvidenceObject(evidenceID, EvidenceRetentionSeconds(), publish)
	} else {
		err = publish()
	}
	if err != nil {
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
		return nil, ErrEvidenceIntegrityFailed
	}
	plaintext, err := s.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return nil, ErrEvidenceDecryptFailed
	}
	return plaintext, nil
}

func (s *localEncryptedEvidenceStore) Delete(key string) (int64, error) {
	path, err := s.resolvePath(key)
	if err != nil {
		return 0, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// 对象已不存在视为幂等成功，不重复计入释放空间。
			return 0, nil
		}
		return 0, fmt.Errorf("evidence object stat failed: %w", err)
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("evidence object delete failed: %w", err)
	}
	return info.Size(), nil
}

func (s *localEncryptedEvidenceStore) RemoveEmptyEvidenceDir(evidenceId int64) error {
	if evidenceId <= 0 {
		return nil
	}
	// 安全范围只到单条证据目录（<root>/<id>），不递归删除数据根目录。
	path := filepath.Join(s.baseDir, strconv.FormatInt(evidenceId, 10))
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		entries, readErr := os.ReadDir(path)
		if readErr == nil && len(entries) > 0 {
			return &EvidenceDirectoryNotEmptyError{Files: len(entries)}
		}
	}
	return err
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
