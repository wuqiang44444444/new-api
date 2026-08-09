package provider_exposure_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActiveForImplementationRequiresEnabledExactIdentity(t *testing.T) {
	setting := PolicySetting{Enabled: true, MonitoredImplementations: "provider.one/v1,provider.two/v2"}
	assert.True(t, setting.ActiveForImplementation("provider.one", "v1"))
	assert.False(t, setting.ActiveForImplementation("provider.one", "v2"))
	setting.Enabled = false
	assert.False(t, setting.ActiveForImplementation("provider.one", "v1"))
}

func TestDefaultPolicyMonitorsBothSelectableFeicaiImplementations(t *testing.T) {
	assert.True(t, policySetting.MonitorsImplementation("feicai.seedance-videos", "v2"))
	assert.True(t, policySetting.MonitorsImplementation("feicai.seedance-videos", "v3"))
}
