package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSynlinkPublicErrorPreservesCustomerAlias(t *testing.T) {
	assert.Equal(t, "video service failed for customer-synlink on requested model [redacted]", PublicTaskErrorMessageForModel("Synlink failed for customer-synlink on private-model api_key=fixture-secret", "customer-synlink", "private-model"))
	assert.Equal(t, "video service failed", PublicTaskErrorMessage("SYNLINK failed"))
	assert.Empty(t, PublicTaskErrorCode("Synlink.Failed"))
}
