package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCreateRequestHashCanonicalizesJSON(t *testing.T) {
	first := taskCreateDigestContext(t, "application/json", []byte(`{"model":"m","prompt":"p","seed":9007199254740993}`))
	second := taskCreateDigestContext(t, "application/json", []byte(`{"seed":9007199254740993,"prompt":"p","model":"m"}`))

	firstHash, err := taskCreateRequestHash(first, "modelark_v3")
	require.NoError(t, err)
	secondHash, err := taskCreateRequestHash(second, "modelark_v3")
	require.NoError(t, err)

	assert.Equal(t, firstHash, secondHash)

	different := taskCreateDigestContext(t, "application/json", []byte(`{"seed":9007199254740992,"prompt":"p","model":"m"}`))
	differentHash, err := taskCreateRequestHash(different, "modelark_v3")
	require.NoError(t, err)
	assert.NotEqual(t, firstHash, differentHash)
}

func taskCreateDigestContext(t *testing.T, contentType string, body []byte) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", contentType)
	return context
}
