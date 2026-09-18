package operation_setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseErrorReportRecipients(t *testing.T) {
	assert.Empty(t, ParseErrorReportRecipients(""))
	got := ParseErrorReportRecipients(" a@example.com ; b@example.com,  a@example.com \n c@example.com")
	assert.Equal(t, []string{"a@example.com", "b@example.com", "c@example.com"}, got, "去空白并去重，保持首次出现顺序")
}

func TestValidateErrorReportRecipients(t *testing.T) {
	got, err := ValidateErrorReportRecipients("ops@example.com;ops@example.com")
	require.NoError(t, err)
	assert.Equal(t, []string{"ops@example.com"}, got)

	_, err = ValidateErrorReportRecipients("bad-address")
	assert.Error(t, err)

	_, err = ValidateErrorReportRecipients("a@example.com\r\nBcc: attacker@example.com")
	assert.Error(t, err, "必须拒绝 CR/LF 注入")

	_, err = ValidateErrorReportRecipients(`"local part" <quoted@example.com>`)
	assert.Error(t, err, "显示名形式会被规范化改变，必须拒绝而不是改写")
}

func TestValidateErrorReportRecipientsLongLocalPart(t *testing.T) {
	long := strings.Repeat("a", 100) + "@example.com"
	got, err := ValidateErrorReportRecipients(long)
	require.NoError(t, err)
	assert.Equal(t, []string{long}, got)
}
