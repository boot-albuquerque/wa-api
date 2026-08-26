// Package apidocs serves the OpenAPI specification and the Swagger UI that
// reads it.
//
// WHY THE ASSETS ARE VENDORED AND EMBEDDED. A documentation page that fetches
// its own JavaScript from a CDN stops working the day the CDN does, pins the
// deployment to an outbound connection it may not have, and hands a third
// party the ability to change what runs inside the operator's browser on a
// page that carries their API token. The dist of swagger-ui 5.32.14 lives in
// swaggerui/ and goes into the binary.
//
// WHY THE PAGE IS UNAUTHENTICATED. The specification describes the shape of
// the API; it carries no credential and no data. The token is supplied by the
// reader through the Authorize button and travels only on the requests they
// make. Putting the page behind the token would mean needing a token to learn
// how to obtain one.
package apidocs

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed swaggerui
var swaggerAssets embed.FS

//go:embed openapi.yaml
var specification []byte

// BasePath is the prefix the router mounts this package on.
const BasePath = "/docs"

// SpecPath is where the raw specification is served, and what index.html asks
// Swagger UI to load.
const SpecPath = BasePath + "/openapi.yaml"

// SwaggerUIVersion is the vendored release. Kept as a constant so an operator
// can read it from the code without unpacking the binary, and so upgrading it
// is a diff that mentions the version.
const SwaggerUIVersion = "5.32.14"

const (
	contentTypeHeader = "Content-Type"
	cacheHeader       = "Cache-Control"
	yamlContentType   = "application/yaml; charset=utf-8"
	htmlContentType   = "text/html; charset=utf-8"
	assetCachePolicy  = "public, max-age=3600"
	specCachePolicy   = "no-cache"
	assetsDir         = "swaggerui"
)

// Specification returns the embedded document. Exported so tests can assert
// against the very bytes the server hands out, rather than re-reading the file
// from disk and proving nothing about what was built.
func Specification() []byte { return specification }

// Handler serves the Swagger UI page, its assets and the specification.
func Handler() http.Handler {
	assets, err := fs.Sub(swaggerAssets, assetsDir)
	if err != nil {
		// Impossible unless the embed directive and the constant disagree,
		// which is a build-time mistake and not a runtime condition.
		panic("apidocs: embedded assets missing: " + err.Error())
	}
	fileServer := http.StripPrefix(BasePath+"/", http.FileServer(http.FS(assets)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == BasePath || r.URL.Path == BasePath+"/":
			w.Header().Set(contentTypeHeader, htmlContentType)
			_, _ = w.Write([]byte(indexHTML))
		case r.URL.Path == SpecPath:
			w.Header().Set(contentTypeHeader, yamlContentType)
			w.Header().Set(cacheHeader, specCachePolicy)
			_, _ = w.Write(specification)
		default:
			if strings.Contains(r.URL.Path, "..") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set(cacheHeader, assetCachePolicy)
			fileServer.ServeHTTP(w, r)
		}
	})
}

// indexHTML is written here rather than served from the dist because the dist
// index points at petstore.swagger.io and would have to be patched anyway.
//
// persistAuthorization keeps the token across reloads: without it, every page
// refresh during a debugging session throws the credential away, and the
// operator retypes it.
const indexHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>wa-api — documentação da API</title>
  <link rel="stylesheet" href="` + BasePath + `/swagger-ui.css">
  <link rel="icon" type="image/png" href="` + BasePath + `/favicon-32x32.png" sizes="32x32">
  <style>
    body { margin: 0; background: #fafafa; }
    .swagger-ui .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="` + BasePath + `/swagger-ui-bundle.js" charset="UTF-8"></script>
  <script src="` + BasePath + `/swagger-ui-standalone-preset.js" charset="UTF-8"></script>
  <script>
    window.onload = function () {
      window.ui = SwaggerUIBundle({
        url: "` + SpecPath + `",
        dom_id: "#swagger-ui",
        deepLinking: true,
        docExpansion: "none",
        defaultModelsExpandDepth: 1,
        defaultModelRendering: "example",
        displayRequestDuration: true,
        filter: true,
        persistAuthorization: true,
        tryItOutEnabled: true,
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        plugins: [SwaggerUIBundle.plugins.DownloadUrl],
        layout: "StandaloneLayout",
        oauth2RedirectUrl: window.location.origin + "` + BasePath + `/oauth2-redirect.html"
      });
    };
  </script>
</body>
</html>
`
