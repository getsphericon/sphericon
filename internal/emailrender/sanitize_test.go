package emailrender_test

import (
	"testing"

	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each vector is hostile markup wrapped in an otherwise fine body; gone lists what
// must not survive the pass, kept what must.
func TestSanitizeHTMLNeutralisesAttackVectors(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		gone []string
		kept []string
	}{
		"script":                  {`<p>hi</p><script>alert(1)</script>`, []string{"<script", "alert(1)"}, []string{"<p>hi</p>"}},
		"script in a table cell":  {`<table><tr><td><script>alert(1)</script>x</td></tr></table>`, []string{"<script", "alert"}, []string{"x"}},
		"iframe":                  {`<iframe src="https://evil.example/"></iframe><p>hi</p>`, []string{"<iframe", "evil.example"}, []string{"<p>hi</p>"}},
		"form and input":          {`<form action="https://evil.example"><input name="pw"><button>go</button></form>`, []string{"<form", "<input", "evil.example", "<button"}, nil},
		"svg onload":              {`<svg onload="alert(1)"><circle/></svg><p>hi</p>`, []string{"<svg", "onload", "alert"}, []string{"<p>hi</p>"}},
		"object and embed":        {`<object data="x.swf"></object><embed src="x.swf"><p>hi</p>`, []string{"<object", "<embed", "x.swf"}, nil},
		"base":                    {`<base href="https://evil.example/"><p>hi</p>`, []string{"<base", "evil.example"}, nil},
		"stylesheet link":         {`<link rel="stylesheet" href="https://evil.example/a.css"><p>hi</p>`, []string{"<link", "evil.example"}, nil},
		"event handler":           {`<p onclick="alert(1)" onmouseover="x()">hi</p>`, []string{"onclick", "onmouseover", "alert"}, []string{"hi"}},
		"event handler on img":    {`<img src="https://example.com/a.png" onerror="alert(1)">`, []string{"onerror", "alert"}, []string{`src="https://example.com/a.png"`}},
		"javascript url":          {`<a href="javascript:alert(1)">x</a>`, []string{"javascript", "alert"}, []string{"x"}},
		"javascript url mixed":    {`<a href="JaVaScRiPt:alert(1)">x</a>`, []string{"avascript", "alert"}, nil},
		"javascript url split":    {"<a href=\"java&#9;script:alert(1)\">x</a>", []string{"script:", "alert"}, nil},
		"javascript url encoded":  {`<a href="&#106;avascript:alert(1)">x</a>`, []string{"avascript", "alert"}, nil},
		"data html url":           {`<a href="data:text/html;base64,PHNjcmlwdD4=">x</a>`, []string{"data:", "base64"}, nil},
		"data image url":          {`<img src="data:image/svg+xml;base64,PHN2Zz4=">`, []string{"data:"}, nil},
		"vbscript url":            {`<a href="vbscript:msgbox(1)">x</a>`, []string{"vbscript"}, nil},
		"css expression":          {`<p style="width:expression(alert(1))">x</p>`, []string{"expression", "alert"}, []string{"x"}},
		"css javascript url":      {`<p style="background:url(javascript:alert(1))">x</p>`, []string{"javascript", "alert"}, nil},
		"css escaped javascript":  {`<p style="background:url(\6a avascript:alert(1))">x</p>`, []string{"avascript", "alert"}, nil},
		"css behavior":            {`<p style="behavior:url(x.htc)">x</p>`, []string{"x.htc"}, nil},
		"style import":            {`<style>@import url(https://evil.example/a.css); p{color:red}</style><p>hi</p>`, []string{"@import", "evil.example"}, []string{"<p>hi</p>"}},
		"style breakout":          {`<style>p{color:red}</style><script>alert(1)</script>`, []string{"<script", "alert"}, nil},
		"style script in comment": {`<style>/* </style><script>alert(1)</script> */</style>`, []string{"<script", "alert"}, nil},
		"style javascript url":    {`<style>p{background:url("javascript:alert(1)")}</style>`, []string{"javascript", "alert"}, nil},
		"style expression":        {`<style>p{width:expression(alert(1))}</style>`, []string{"expression", "alert"}, nil},
		"style escape":            {`<style>p{background:\75rl(javascript:alert(1))}</style>`, []string{"alert"}, nil},
		"script in mso comment":   {`<!--[if mso]><script>alert(1)</script><p>mso</p><![endif]-->`, []string{"<script", "alert"}, []string{"<p>mso</p>", "<!--[if mso]>", "<![endif]-->"}},
		"forged condition":        {`<!--[if mso]><img src=x onerror=alert(1)>--><p>x</p>`, []string{"onerror", "alert"}, nil},
		"comment payload":         {`<!-- <script>alert(1)</script> --><p>hi</p>`, []string{"<script", "alert", "<!--"}, []string{"<p>hi</p>"}},
		"bad condition text":      {`<!--[if mso"><script>alert(1)</script>]><![endif]--><p>hi</p>`, []string{"<script", "alert"}, nil},
		"vml onclick":             {`<!--[if mso]><v:roundrect onclick="alert(1)" href="https://example.com/a"><w:anchorlock/></v:roundrect><![endif]-->`, []string{"onclick", "alert"}, []string{`href="https://example.com/a"`}},
		"vml javascript href":     {`<!--[if mso]><v:roundrect href="javascript:alert(1)"></v:roundrect><![endif]-->`, []string{"javascript", "alert"}, nil},
		"xmp":                     {`<xmp><img src=x onerror=alert(1)></xmp>`, []string{"<img", "onerror"}, nil},
		"noscript":                {`<noscript><p title="</noscript><img src=x onerror=alert(1)>"></noscript>`, []string{"onerror"}, nil},
		"title breakout":          {`<title><img src=x onerror=alert(1)></title>`, []string{"<img"}, nil},
		"meta refresh":            {`<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`, []string{"http-equiv"}, nil},
		"style attribute break":   {`<p style="color:red;" onclick="alert(1)">x</p>`, []string{"onclick", "alert"}, nil},
		"forged placeholder":      {`<p>sph0000000000000000C0x sph0000000000000000L7x sph0000000000000000S99x</p>`, nil, []string{"sph0000000000000000C0x", "sph0000000000000000L7x"}},
		"forged sph element":      {`<sph-hc i="999"></sph-hc><p>hi</p>`, []string{"<sph-hc"}, []string{"<p>hi</p>"}},
		"unterminated tag":        {`<p>hi</p><img src="x" onerror="alert(1)`, []string{"onerror", "alert"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			out := emailrender.SanitizeHTML(tc.in)
			for _, g := range tc.gone {
				assert.NotContains(t, out, g)
			}
			for _, k := range tc.kept {
				assert.Contains(t, out, k)
			}
			assert.Equal(t, out, emailrender.SanitizeHTML(out), "a second pass changes nothing")
		})
	}
}

// What an email needs must come through the pass unchanged.
func TestSanitizeHTMLKeepsEmailMarkup(t *testing.T) {
	for name, in := range map[string]string{
		"table with inline styles": `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" bgcolor="#ffffff" style="background-color:#ffffff;width:100%;max-width:600px;border-collapse:collapse;"><tbody><tr><td align="center" class="px" style="padding:24px 16px;font-family:Arial,sans-serif;font-size:16px;line-height:24px;color:#222222">x</td></tr></tbody></table>`,
		"style block":              `<style>@media (max-width:600px){.px{padding:8px !important}} a{color:#0a58ca}</style><p>x</p>`,
		"style block with media":   `<style media="screen and (min-width:600px)">.a{color:red}</style><p>x</p>`,
		"mso conditional":          `<!--[if mso]><table role="presentation"><tr><td width="600"><![endif]--><p>x</p><!--[if mso]></td></tr></table><![endif]-->`,
		"revealed conditional":     `<!--[if !mso]><!--><p>modern</p><!--<![endif]-->`,
		"vml button":               `<!--[if mso]><v:roundrect xmlns:v="urn:schemas-microsoft-com:vml" href="https://example.com/go" style="height:40px;v-text-anchor:middle;width:200px" arcsize="10%" fillcolor="#0a58ca"><w:anchorlock/><center>Go</center></v:roundrect><![endif]-->`,
		"merge tags in text":       `<p>Hello {{ first_name }}, {{ plan | default: "free" }}</p>`,
		"merge tags in urls":       `<a href="https://example.com/u/{{ id }}?a={{ a }}">x</a><a href="{{ unsubscribe_url }}">y</a><a href="mailto:{{ email }}">z</a><img src="{{ logo }}" alt="l">`,
		"liquid between rows":      `<table><tbody>{% for i in items %}<tr><td>{{ i }}</td></tr>{% endfor %}</tbody></table>`,
		"liquid in style":          `<p style="color:{{ color }};padding:0 {{ pad }}px">x</p>`,
		"liquid in style block":    `<style>a{color:{{ brand }}}</style><p>x</p>`,
		"liquid around a table":    `{% if show %}<table><tbody><tr><td>x</td></tr></tbody></table>{% endif %}`,
		"liquid in conditional":    `<!--[if mso]><v:roundrect href="{{ cta }}"></v:roundrect><![endif]-->`,
		"background image style":   `<td style="background-image:url(https://example.com/bg.png);background-color:#fff">x</td>`,
		"image and link":           `<a href="https://example.com/sale" target="_blank"><img src="https://example.com/logo.png" alt="Logo" width="120" height="40" style="display:block;border:0"></a>`,
		"head":                     `<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Hi</title></head><body><p>x</p></body></html>`,
		"data attribute":           `<a href="https://example.com" data-track="cta">x</a>`,
		"entities":                 `<p>Fish &amp; chips &lt;3</p>`,
		"mso properties":           `<td style="mso-line-height-rule:exactly;mso-table-lspace:0pt;mso-table-rspace:0pt">x</td>`,
		"preheader":                `<div style="display:none;max-height:0;overflow:hidden">Preview text</div>`,
		"doctype":                  `<!DOCTYPE html><html><head></head><body><p>x</p></body></html>`,
		"outlook xml settings":     `<!--[if gte mso 9]><xml><o:OfficeDocumentSettings><o:AllowPNG/><o:PixelsPerInch>96</o:PixelsPerInch></o:OfficeDocumentSettings></xml><![endif]-->`,
		"conditional with styles":  `<!--[if mso]><style>td{font-family:Arial}</style><![endif]-->`,
		"merge tag as whole style": `<p style="{{ style }}">x</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, in, emailrender.SanitizeHTML(in))
		})
	}
}

func TestSanitizeHTMLRestoresMergeTagsExactly(t *testing.T) {
	in := `<p>{{ a }}</p>{%- if b -%}<a href="{{ u }}">x</a>{%- endif -%}{% comment %}note{% endcomment %}`
	out := emailrender.SanitizeHTML(in)
	require.Equal(t, in, out)
}

func TestSanitizeHTMLEmpty(t *testing.T) {
	assert.Empty(t, emailrender.SanitizeHTML(""))
}

// A quoted font name is the same style once the quote is written as an entity.
func TestSanitizeHTMLKeepsQuotedFontFamily(t *testing.T) {
	out := emailrender.SanitizeHTML(`<td style="font-family:'Helvetica Neue',Arial">x</td>`)
	assert.Contains(t, out, `font-family:&#39;Helvetica Neue&#39;,Arial`)
}

func TestRenderEmailEscapesVariablesInTheBody(t *testing.T) {
	out, err := emailrender.RenderEmail("Hi {{ first_name }}", `<p>Hello {{ first_name }}</p>`,
		map[string]any{"first_name": `<img src=x onerror=alert(1)> & "Co"`})
	require.NoError(t, err)

	assert.Contains(t, out.HTML, `<p>Hello &lt;img src=x onerror=alert(1)&gt; &amp; &#34;Co&#34;</p>`, "markup is shown as text")
	assert.NotContains(t, out.HTML, "<img")
	assert.Equal(t, `Hi <img src=x onerror=alert(1)> & "Co"`, out.Subject, "the subject is plain text with no entities")
}

func TestRenderEmailSanitizesWhatLiquidProduced(t *testing.T) {
	out, err := emailrender.RenderEmail("s", `<a href="{{ link }}">go</a><p style="color:{{ color }}">x</p>`,
		map[string]any{"link": "javascript:alert(1)", "color": "red;background:url(javascript:alert(1))"})
	require.NoError(t, err)

	assert.NotContains(t, out.HTML, "javascript")
	assert.NotContains(t, out.HTML, "alert")
	assert.Contains(t, out.HTML, "go")
}

func TestRenderEmailSanitizesMarkupThatLiquidOutputsOnPurpose(t *testing.T) {
	vars := map[string]any{"block": `<b>bold</b><script>alert(1)</script>`}

	escaped, err := emailrender.RenderEmail("s", `<p>{{ block }}</p>`, vars)
	require.NoError(t, err)
	assert.Contains(t, escaped.HTML, "&lt;b&gt;bold&lt;/b&gt;")

	raw, err := emailrender.RenderEmail("s", `<p>{{ block | safe }}</p>`, vars)
	require.NoError(t, err)
	assert.Contains(t, raw.HTML, "<p><b>bold</b></p>", "the raw output keeps the markup the variable carries on purpose")
	assert.NotContains(t, raw.HTML, "script", "and the second pass still strips what no email may carry")
}

func TestRenderEmailKeepsLiquidControlFlowAroundTableRows(t *testing.T) {
	body := `<table><tbody>{% for i in items %}<tr><td>{{ i }}</td></tr>{% endfor %}</tbody></table>`
	out, err := emailrender.RenderEmail("s", body, map[string]any{"items": []string{"a", "b"}})
	require.NoError(t, err)
	assert.Contains(t, out.HTML, "<tr><td>a</td></tr><tr><td>b</td></tr>")
}

func TestPrepareFieldsSanitizesTheBodyToStore(t *testing.T) {
	subject, body := "Hi {{ first_name }}", `<p onclick="x()">Hello {{ first_name }}</p><script>alert(1)</script>`
	clean, err := emailrender.PrepareFields(&subject, &body)
	require.NoError(t, err)
	require.NotNil(t, clean)
	assert.Equal(t, `<p>Hello {{ first_name }}</p>`, *clean)

	none, err := emailrender.PrepareFields(&subject, nil)
	require.NoError(t, err)
	assert.Nil(t, none, "a body that was not provided stays not provided")
}

func TestPrepareFieldsRefusesWhatCannotRender(t *testing.T) {
	broken := `<p>hi</p><div class="a"`
	_, err := emailrender.PrepareFields(nil, &broken)
	require.ErrorIs(t, err, emailrender.ErrInvalidContent)
}
