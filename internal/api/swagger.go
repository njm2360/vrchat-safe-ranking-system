package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	apidocs "github.com/njm2360/vrchat-ranking-system/api"
)

func handleOpenapiSpec(c echo.Context) error {
	return c.Blob(http.StatusOK, "application/yaml", apidocs.Spec)
}

func handleSwaggerUI(c echo.Context) error {
	html := `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>VRChat Ranking System API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"/>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: "/openapi.yaml",
      dom_id: "#swagger-ui",
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      layout: "BaseLayout",
    });
  </script>
</body>
</html>`
	return c.HTML(http.StatusOK, html)
}
