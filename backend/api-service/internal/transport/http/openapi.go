package http

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
)

const (
	swaggerUIVersion   = "5.33.0"
	swaggerUICSSSRI    = "sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW"
	swaggerUIBundleSRI = "sha384-YDALVcy8kj8yltLBVi1vBiBAUqdxvus673gM8XKwiy6aDUJFXivF/KCufekjYbVf"
	swaggerUIOrigin    = "https://cdn.jsdelivr.net"
)

const swaggerInitScript = `
      window.onload = function () {
        window.ui = SwaggerUIBundle({
          url: "/openapi.yaml",
          dom_id: "#swagger-ui",
        });
      };
    `

const swaggerUIPageTemplate = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Арена переговоров — API docs</title>
    <link rel="stylesheet" href="%[1]s/npm/swagger-ui-dist@%[2]s/swagger-ui.css" integrity="%[3]s" crossorigin="anonymous" referrerpolicy="no-referrer" />
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="%[1]s/npm/swagger-ui-dist@%[2]s/swagger-ui-bundle.js" integrity="%[4]s" crossorigin="anonymous" referrerpolicy="no-referrer"></script>
    <script>%[5]s</script>
  </body>
</html>
`

var (
	swaggerUIPage = fmt.Sprintf(
		swaggerUIPageTemplate,
		swaggerUIOrigin,
		swaggerUIVersion,
		swaggerUICSSSRI,
		swaggerUIBundleSRI,
		swaggerInitScript,
	)
	swaggerContentSecurityPolicy = "default-src 'none'; " +
		"script-src '" + inlineScriptHash(swaggerInitScript) + "' " + swaggerUIOrigin + "; " +
		"style-src 'unsafe-inline' " + swaggerUIOrigin + "; " +
		"img-src 'self' data:; " +
		"font-src 'self' data:; " +
		"connect-src 'self'; " +
		"base-uri 'none'; " +
		"form-action 'none'; " +
		"frame-ancestors 'none'"
)

func inlineScriptHash(script string) string {
	sum := sha256.Sum256([]byte(script))

	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

func newOpenAPISpecHandler(spec []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(spec)
	}
}

func swaggerUIHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", swaggerContentSecurityPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write([]byte(swaggerUIPage))
}
