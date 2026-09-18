package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorReportOmitsHTTPBodies(t *testing.T) {
	detail, err := common.Marshal(map[string]string{"upstream_status": "400", "http_exchange": `{"request":{"body":"private business prompt"}}`})
	require.NoError(t, err)
	rendered := strings.Join(renderDetailCell(string(detail), 1024), "")
	assert.Contains(t, rendered, "upstream_status=400")
	assert.NotContains(t, rendered, "private business prompt")
	assert.NotContains(t, rendered, "http_exchange")
}
