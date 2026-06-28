package com.aandi.gateway.errorcontract

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponseWriter
import com.aandi.gateway.logging.ApiLogContext
import com.aandi.gateway.logging.ApiLogFactory
import com.aandi.gateway.logging.ApiLoggingProperties
import com.aandi.gateway.logging.ApiStructuredLogger
import com.aandi.gateway.logging.GlobalExceptionHandler
import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.context.ApplicationContext
import org.springframework.http.MediaType
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.security.core.authority.SimpleGrantedAuthority
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.mockJwt
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.springSecurity
import org.springframework.test.web.reactive.server.WebTestClient
import tools.jackson.databind.ObjectMapper
import kotlin.test.assertEquals
import kotlin.test.assertTrue

@SpringBootTest(
    properties = [
        "AUTH_SERVICE_URI=http://127.0.0.1:1",
        "POST_SERVICE_URI=http://127.0.0.1:1",
        "ONLINE_JUDGE_SERVICE_URI=http://127.0.0.1:1",
        "app.security.internal-event-token=test-internal-token",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!",
        "app.security.policy.enforce-https=false"
    ]
)
class GatewayErrorContractTest(
    @Autowired private val applicationContext: ApplicationContext
) {
    private val webTestClient: WebTestClient by lazy {
        WebTestClient.bindToApplicationContext(applicationContext)
            .apply(springSecurity())
            .configureClient()
            .build()
    }

    @Test
    fun `authentication failure returns standard 401 envelope with trace headers`() {
        webTestClient.get()
            .uri("/v1/me")
            .exchange()
            .expectGatewayError(GatewayErrorCode.AUTHENTICATION_FAILED)
    }

    @Test
    fun `authorization failure returns standard 403 envelope with trace headers`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .get()
            .uri("/v1/admin/ping")
            .exchange()
            .expectGatewayError(GatewayErrorCode.ACCESS_DENIED)
    }

    @Test
    fun `allowlist failure returns standard 404 envelope with trace headers`() {
        webTestClient.get()
            .uri("/not-allowlisted-for-observability")
            .exchange()
            .expectGatewayError(GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED)
    }

    @Test
    fun `content type failure returns standard 415 envelope with trace headers`() {
        webTestClient.post()
            .uri("/v1/auth/login")
            .contentType(MediaType.TEXT_PLAIN)
            .bodyValue("username=demo")
            .exchange()
            .expectGatewayError(GatewayErrorCode.JSON_CONTENT_TYPE_REQUIRED)
    }

    @Test
    fun `rate limit failure returns standard 429 envelope with trace headers`() {
        val body = """{"username":"observability-rate-limit","password":"demo"}"""
        repeat(10) {
            webTestClient.post()
                .uri("/v1/auth/login")
                .contentType(MediaType.APPLICATION_JSON)
                .bodyValue(body)
                .exchange()
        }

        webTestClient.post()
            .uri("/v1/auth/login")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue(body)
            .exchange()
            .expectGatewayError(GatewayErrorCode.LOGIN_RATE_LIMIT_EXCEEDED)
    }

    @Test
    fun `downstream connection failure returns standard 502 envelope with trace headers`() {
        webTestClient.get()
            .uri("/v2/ping")
            .exchange()
            .expectGatewayError(GatewayErrorCode.DOWNSTREAM_SERVICE_UNAVAILABLE)
    }

    @Test
    fun `unknown gateway exception returns standard 500 envelope with trace headers`() {
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.get("/v2/internal-error")
                .header(ApiLogContext.TRACE_ID_HEADER, "trace-error-contract")
                .header(ApiLogContext.REQUEST_ID_HEADER, "request-error-contract")
                .build()
        )
        val context = ApiLogContext.initialize(exchange)
        val handler = GlobalExceptionHandler(
            responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*"),
            apiLogFactory = ApiLogFactory(
                objectMapper = ObjectMapper(),
                properties = ApiLoggingProperties(
                    env = "test",
                    service = ApiLoggingProperties.ServiceProperties(
                        name = "gateway",
                        domain = "gateway",
                        domainCode = 1,
                        version = "test",
                        instanceId = "test"
                    )
                )
            ),
            apiStructuredLogger = ApiStructuredLogger()
        )

        handler.handle(exchange, IllegalStateException("unexpected gateway error")).block()

        val body = context.responseBody
        assertEquals(GatewayErrorCode.INTERNAL_SERVER_ERROR.httpStatus, exchange.response.statusCode)
        assertEquals("trace-error-contract", exchange.response.headers.getFirst(ApiLogContext.TRACE_ID_HEADER))
        assertEquals("request-error-contract", exchange.response.headers.getFirst(ApiLogContext.REQUEST_ID_HEADER))
        val root = ObjectMapper().readTree(body)
        assertEquals(false, root.path("success").asBoolean())
        assertTrue(root.path("data").isNull)
        assertEquals(GatewayErrorCode.INTERNAL_SERVER_ERROR.code, root.path("error").path("code").asInt())
        assertEquals(GatewayErrorCode.INTERNAL_SERVER_ERROR.value, root.path("error").path("value").asText())
        assertEquals(GatewayErrorCode.INTERNAL_SERVER_ERROR.message, root.path("error").path("message").asText())
        assertTrue(root.path("timestamp").asText().isNotBlank())
    }

    private fun WebTestClient.ResponseSpec.expectGatewayError(errorCode: GatewayErrorCode) {
        expectStatus().isEqualTo(errorCode.httpStatus.value())
            .expectHeader().contentTypeCompatibleWith(MediaType.APPLICATION_JSON)
            .expectHeader().exists(ApiLogContext.TRACE_ID_HEADER)
            .expectHeader().exists(ApiLogContext.REQUEST_ID_HEADER)
            .expectBody()
            .jsonPath("$.success").isEqualTo(false)
            .jsonPath("$.data").isEmpty()
            .jsonPath("$.error.code").isEqualTo(errorCode.code)
            .jsonPath("$.error.value").isEqualTo(errorCode.value)
            .jsonPath("$.error.message").isEqualTo(errorCode.message)
            .jsonPath("$.error.alert").isEqualTo(errorCode.alert)
            .jsonPath("$.timestamp").exists()
    }
}
