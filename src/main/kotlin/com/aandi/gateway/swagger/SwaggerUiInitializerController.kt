package com.aandi.gateway.swagger

import org.springframework.beans.factory.annotation.Value
import org.springframework.http.MediaType
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RestController

@RestController
class SwaggerUiInitializerController(
    @Value("\${springdoc.swagger-ui.config-url:/v3/api-docs/swagger-config}")
    private val configUrl: String,
    @Value("\${springdoc.swagger-ui.layout:StandaloneLayout}")
    private val layout: String
) {

    @GetMapping(
        "/swagger-ui/swagger-initializer.js",
        "/v2/swagger-ui/swagger-initializer.js",
        produces = ["application/javascript"]
    )
    fun swaggerInitializer(): String = """
        window.onload = function() {
          window.ui = SwaggerUIBundle({
            configUrl: "$configUrl",
            dom_id: '#swagger-ui',
            deepLinking: true,
            presets: [
              SwaggerUIBundle.presets.apis,
              SwaggerUIStandalonePreset
            ],
            plugins: [
              SwaggerUIBundle.plugins.DownloadUrl
            ],
            layout: "$layout",
            requestInterceptor: function(request) {
              if (!request.headers) {
                request.headers = {};
              }
              const authenticate = request.headers["Authenticate"] || request.headers["authenticate"];
              if (typeof authenticate === "string") {
                const trimmed = authenticate.trim();
                if (trimmed.length > 0 && !/^Bearer\s+/i.test(trimmed)) {
                  request.headers["Authenticate"] = "Bearer " + trimmed;
                }
              }
              return request;
            }
          });
        };
    """.trimIndent()
}
