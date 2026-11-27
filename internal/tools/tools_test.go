package tools

import (
	"strings"
	"testing"
)

func TestHTMLToTextDropsScriptStyleNoScript(t *testing.T) {
	html := `<html><head>
		<style>.a{color:red}</style>
		<script>var secret = 1 < 2;</script>
		<noscript>Enable JavaScript</noscript>
	</head><body>
		<h1>Solar&nbsp;System</h1>
		<p>The Sun &amp; planets.</p>
	</body></html>`

	got := htmlToText(html)
	if strings.Contains(got, "color:red") {
		t.Fatalf("style content leaked: %q", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("script content leaked: %q", got)
	}
	if strings.Contains(got, "Enable JavaScript") {
		t.Fatalf("noscript content leaked: %q", got)
	}
	if !strings.Contains(got, "Solar System") {
		t.Fatalf("text missing: %q", got)
	}
	if !strings.Contains(got, "The Sun & planets.") {
		t.Fatalf("entities not decoded: %q", got)
	}
}

func TestHTMLToTextDecodesNumericEntities(t *testing.T) {
	got := htmlToText("<p>&#x2600; and &#9731;</p>")
	if !strings.Contains(got, "\u2600 and \u2603") {
		t.Fatalf("numeric entities not decoded: %q", got)
	}
}

func TestHTMLToTextCollapsesWhitespace(t *testing.T) {
	got := htmlToText("<div>  line\n\tone  </div>\n<div>line two</div>")
	if got != "line one line two" {
		t.Fatalf("unexpected whitespace collapse: %q", got)
	}
}

func TestHTMLToTextDecodesUppercaseHexEntity(t *testing.T) {
	got := htmlToText("<p>&#X2600;</p>")
	if !strings.Contains(got, "\u2600") {
		t.Fatalf("uppercase hex entity not decoded: %q", got)
	}
}
