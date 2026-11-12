package service

import (
	"fmt"
	"html"
	"strings"
)

// The outer table and inline styles keep the layout readable in Outlook and
// other mail clients that strip embedded styles. Body fragments are rendered
// by the operations templates from escaped data.
func renderOperationsMail(title, bodyHTML string) string {
	return `<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head>
<body style="margin:0;background:#eef2f6;color:#17263a;font-family:Arial,'Microsoft YaHei',sans-serif">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#eef2f6">
<tr><td align="center" style="padding:24px 12px">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:640px;background:#ffffff;border:1px solid #dce4ee">
<tr><td style="background:#14283f;border-top:4px solid #1b9b8e;padding:22px 24px">
<h2 style="margin:0;color:#ffffff;font-size:21px;line-height:1.4">` + html.EscapeString(title) + `</h2>
</td></tr>
<tr><td style="padding:20px 24px;color:#26394f;font-size:14px;line-height:1.6;overflow-wrap:anywhere">` + bodyHTML + `</td></tr>
</table>
</td></tr></table></body></html>`
}

// Each row is a text label and value. These tables are read as data by screen readers.
func writeOperationsMailRows(body *strings.Builder, rows [][2]string) {
	body.WriteString(`<table width="100%" cellspacing="0" cellpadding="0" style="margin:10px 0 18px;border-collapse:collapse;table-layout:fixed">`)
	for _, row := range rows {
		fmt.Fprintf(body, `<tr><th scope="row" align="left" width="48%%" style="padding:9px 10px;background:#f2f6fa;border-bottom:1px solid #dce4ee;color:#334c65;font-weight:600">%s</th><td style="padding:9px 10px;border-bottom:1px solid #dce4ee;overflow-wrap:anywhere">%s</td></tr>`,
			html.EscapeString(row[0]), html.EscapeString(row[1]))
	}
	body.WriteString("</table>")
}
