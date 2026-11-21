package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerExportArchiveCompressesPublishedParts(t *testing.T) {
	job, store := customerExportArchiveFixture(t)
	archive, err := PrepareCustomerExportArchive(job)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, archive.Write(t.Context(), &output))

	files, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	require.NoError(t, err)
	require.Len(t, files.File, 2)
	for _, entry := range files.File {
		assert.EqualValues(t, zip.Deflate, entry.Method)
		body, err := entry.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, body.Close())
		assert.Equal(t, store.uploaded[CustomerExportObjectNamespace+"/"+job.JobID+"/"+entry.Name], content)
	}
	assert.Equal(t, "statement-details.csv", files.File[0].Name)
	assert.Equal(t, "statement-details.part2.csv", files.File[1].Name)
}

func TestCustomerExportArchiveDoesNotPublishIfAnObjectHasChanged(t *testing.T) {
	job, store := customerExportArchiveFixture(t)
	archive, err := PrepareCustomerExportArchive(job)
	require.NoError(t, err)
	store.uploaded[CustomerExportObjectNamespace+"/"+job.JobID+"/statement-details.part2.csv"] = []byte("quota\n11\n")

	var output bytes.Buffer
	require.ErrorIs(t, archive.Write(t.Context(), &output), ErrCustomerExportArchiveUnavailable)
	_, err = zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	require.Error(t, err)
}

func customerExportArchiveFixture(t *testing.T) (*model.CustomerExportJob, *stubExportStore) {
	t.Helper()
	job := &model.CustomerExportJob{JobID: "cex_zip_fixture"}
	store := &stubExportStore{uploaded: map[string][]byte{}}
	previous := customerExportStoreOverride
	customerExportStoreOverride = store
	t.Cleanup(func() { customerExportStoreOverride = previous })

	artifact := model.CustomerExportArtifact{StoreIdentity: store.ExportIdentity()}
	for _, name := range []string{"statement-details.csv", "statement-details.part2.csv"} {
		key := CustomerExportObjectNamespace + "/" + job.JobID + "/" + name
		data := []byte("quota\n10\n")
		store.uploaded[key] = data
		artifact.Files = append(artifact.Files, model.CustomerExportArtifactFile{
			FileName: name, ObjectKey: key, SizeBytes: int64(len(data)), Sha256: fmt.Sprintf("%x", sha256.Sum256(data)),
		})
	}
	raw, err := common.Marshal(artifact)
	require.NoError(t, err)
	job.Artifact = string(raw)
	return job, store
}

func TestCustomerExportArchiveTicketRechecksJobAndSession(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	session, err := CreateLoginSession(user.Id, "password", "", "")
	require.NoError(t, err)
	identity, err := ParseAccessToken(session.AccessToken)
	require.NoError(t, err)

	ticket, err := IssueCustomerExportArchiveTicket(identity, "cex_allowed")
	require.NoError(t, err)
	id, err := ValidateCustomerExportArchiveTicket(ticket, "cex_allowed")
	require.NoError(t, err)
	assert.Equal(t, user.Id, id)
	_, err = ValidateCustomerExportArchiveTicket(ticket, "cex_other")
	require.ErrorIs(t, err, ErrAuthTokenInvalid)
	_, err = ParseAccessToken(ticket)
	require.Error(t, err)
	require.NoError(t, model.DB.Model(&model.UserSession{}).Where("sid = ?", identity.SessionID).
		Update("revoked_at", common.GetTimestamp()).Error)
	_, err = ValidateCustomerExportArchiveTicket(ticket, "cex_allowed")
	require.ErrorIs(t, err, ErrLoginSessionRevoked)
}
