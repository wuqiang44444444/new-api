package router

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerExportArchiveRoutesRequireAuthorization(t *testing.T) {
	engine := gin.New()
	api := engine.Group("/api")
	api.Use(apiGzip())
	registerBillingReconciliationRoutes(api)

	for _, endpoint := range []struct{ method, suffix string }{
		{http.MethodPost, "/archive.zip"},
		{http.MethodGet, "/archive.zip"},
		{http.MethodPost, "/share-archive"},
		{http.MethodGet, "/share-link"},
	} {
		t.Run(endpoint.method+endpoint.suffix, func(t *testing.T) {
			request := httptest.NewRequest(endpoint.method, "/api/billing/exports/cex_other"+endpoint.suffix, nil)
			request.Header.Set("Accept-Encoding", "gzip")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, http.StatusUnauthorized, response.Code)
			assert.Empty(t, response.Result().Cookies())
			assert.Empty(t, response.Header().Get("Content-Disposition"))
		})
	}
}

func TestCustomerExportArchiveDownloadsTheOwnersExistingFiles(t *testing.T) {
	setupRelayRouterTestDB(t)
	csv := []byte("quota\n10\n")
	objects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || (r.URL.Path != "/archives/exports/jobs/cex_owned/details.csv" &&
			r.URL.Path != "/archives/exports/jobs/cex_owned/details.part2.csv") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(csv)
	}))
	t.Cleanup(objects.Close)

	priorStorage, err := model.GetObjectStorageSetting()
	require.NoError(t, err)
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate(priorStorage) })
	require.NoError(t, service.SaveObjectStorageSetting(system_setting.ObjectStorageConfig{
		Backend: "s3", Endpoint: objects.URL, Bucket: "archives", Region: "us-east-1",
		AccountName: "fixture", Credential: "fixture-only-secret", Revision: "archive-test",
	}))
	store, ok := service.GetTaskArtifactStore().(interface{ ExportIdentity() string })
	require.True(t, ok)

	owner := model.User{Username: "archive-owner", AffCode: "arcowner", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, model.DB.Create(&owner).Error)
	other := model.User{Username: "archive-other", AffCode: "arcother", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, model.DB.Create(&other).Error)
	ownerSession, err := service.CreateLoginSession(owner.Id, "password", "", "")
	require.NoError(t, err)
	otherSession, err := service.CreateLoginSession(other.Id, "password", "", "")
	require.NoError(t, err)

	expires := common.GetTimestamp() + 300
	artifact := model.CustomerExportArtifact{StoreIdentity: store.ExportIdentity(), ExpiresAt: expires}
	for _, name := range []string{"details.csv", "details.part2.csv"} {
		artifact.Files = append(artifact.Files, model.CustomerExportArtifactFile{
			FileName: name, ObjectKey: "exports/jobs/cex_owned/" + name,
			SizeBytes: int64(len(csv)), Sha256: fmt.Sprintf("%x", sha256.Sum256(csv)),
		})
	}
	raw, err := common.Marshal(artifact)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.CustomerExportJob{
		JobID: "cex_owned", UserId: owner.Id, TargetUserId: owner.Id,
		JobType: model.CustomerExportJobTypeStatementDetails, Status: model.CustomerExportJobStatusSucceeded,
		ExpiresAt: expires, Artifact: string(raw),
	}).Error)

	engine := gin.New()
	api := engine.Group("/api")
	api.Use(apiGzip())
	registerBillingReconciliationRoutes(api)
	path := "/api/billing/exports/cex_owned/archive.zip"
	request := func(method, target, access string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://example.test"+target, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		if access != "" {
			req.Header.Set("Authorization", "Bearer "+access)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	require.Equal(t, http.StatusNotFound, request(http.MethodPost, path, otherSession.AccessToken, nil).Code)
	require.Equal(t, http.StatusNotFound,
		request(http.MethodPost, "/api/billing/exports/cex_owned/share-archive", otherSession.AccessToken, nil).Code)
	require.Equal(t, http.StatusNotFound,
		request(http.MethodGet, "/api/billing/exports/cex_owned/share-link", otherSession.AccessToken, nil).Code)
	grant := request(http.MethodPost, path, ownerSession.AccessToken, nil)
	require.Equal(t, http.StatusNoContent, grant.Code)
	cookies := grant.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, path, cookies[0].Path)
	assert.True(t, cookies[0].HttpOnly)
	assert.True(t, cookies[0].Secure)
	assert.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
	assert.Empty(t, grant.Body.String())
	require.Equal(t, http.StatusUnauthorized,
		request(http.MethodGet, "/api/billing/exports/cex_other/archive.zip", "", cookies[0]).Code)

	response := request(http.MethodGet, path, "", cookies[0])
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/zip", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Header().Get("Content-Disposition"), "attachment")
	assert.Empty(t, response.Header().Get("Content-Encoding"))
	assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	require.NoError(t, err)
	require.Len(t, archive.File, 2)
	assert.Equal(t, "details.csv", archive.File[0].Name)
	assert.Equal(t, "details.part2.csv", archive.File[1].Name)
}
