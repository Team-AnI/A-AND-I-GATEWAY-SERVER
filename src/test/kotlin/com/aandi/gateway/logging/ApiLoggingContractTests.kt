package com.aandi.gateway.logging

import com.aandi.gateway.common.response.GatewayResponseWriter
import org.junit.jupiter.api.Test
import org.springframework.cloud.gateway.support.ServerWebExchangeUtils
import org.springframework.http.HttpStatus
import org.springframework.http.MediaType
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono
import tools.jackson.databind.ObjectMapper
import java.net.ConnectException
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class ApiLoggingContractTests {

    private val factory = ApiLogFactory(
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
    )

    @Test
    fun `api log service includes gateway domain`() {
        val exchange = exchange("/v2/ping")
        val context = ApiLogContext.initialize(exchange)

        val log = factory.create(exchange, context, null)

        assertEquals("gateway", log.service.domain)
        assertEquals("gateway", log.service.toMap()["domain"])
        assertEquals(1, log.service.domainCode)
    }

    @Test
    fun `trace id is reused and propagated`() {
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.get("/v2/ping")
                .header(ApiLogContext.TRACE_ID_HEADER, "trace-1")
                .header(ApiLogContext.REQUEST_ID_HEADER, "request-1")
                .build()
        )
        var propagatedTraceId: String? = null
        var propagatedRequestId: String? = null
        val filter = RequestResponseLoggingFilter(factory, ApiStructuredLogger())
        val chain = WebFilterChain { chainExchange ->
            propagatedTraceId = chainExchange.request.headers.getFirst(ApiLogContext.TRACE_ID_HEADER)
            propagatedRequestId = chainExchange.request.headers.getFirst(ApiLogContext.REQUEST_ID_HEADER)
            chainExchange.response.setComplete()
        }

        filter.filter(exchange, chain).block()

        assertEquals("trace-1", propagatedTraceId)
        assertEquals("request-1", propagatedRequestId)
        assertEquals("trace-1", exchange.response.headers.getFirst(ApiLogContext.TRACE_ID_HEADER))
        assertEquals("request-1", exchange.response.headers.getFirst(ApiLogContext.REQUEST_ID_HEADER))
    }

    @Test
    fun `trace id is generated when absent`() {
        val exchange = exchange("/v2/ping")

        val context = ApiLogContext.initialize(exchange)

        assertTrue(context.traceId.isNotBlank())
        assertEquals(context.traceId, exchange.response.headers.getFirst(ApiLogContext.TRACE_ID_HEADER))
    }

    @Test
    fun `gateway matched path is used as normalized route`() {
        val exchange = exchange("/v2/users/123")
        exchange.attributes[ServerWebExchangeUtils.GATEWAY_PREDICATE_MATCHED_PATH_ATTR] = "/v2/users/{userId}"
        val context = ApiLogContext.initialize(exchange)

        val log = factory.create(exchange, context, null)

        assertEquals("/v2/users/{userId}", log.http.route)
    }

    @Test
    fun `downstream standard error code is preserved in api log`() {
        val exchange = exchange("/v2/auth/me")
        exchange.response.statusCode = HttpStatus.UNAUTHORIZED
        val context = ApiLogContext.initialize(exchange)
        context.responseBody = """
            {
              "success": false,
              "data": null,
              "error": {
                "code": 21101,
                "message": "access token is invalid",
                "value": "ACCESS_TOKEN_INVALID",
                "alert": "login required"
              },
              "timestamp": "2026-05-14T00:00:00+09:00"
            }
        """.trimIndent()

        val log = factory.create(exchange, context, null)

        assertNotNull(log.response.error)
        assertEquals(21101, log.response.error.code)
        assertEquals("ACCESS_TOKEN_INVALID", log.response.error.value)
    }

    @Test
    fun `downstream standard error response body is preserved for client`() {
        val body = """
            {
              "success": false,
              "data": null,
              "error": {
                "code": 44501,
                "message": "assignment has already been submitted",
                "value": "ASSIGNMENT_ALREADY_SUBMITTED",
                "alert": "이미 제출된 과제입니다."
              },
              "timestamp": "2026-05-14T00:00:00+09:00"
            }
        """.trimIndent()
        val exchange = exchange("/v2/reports/assignments/1001/submit")
        val filter = RequestResponseLoggingFilter(factory, ApiStructuredLogger())
        val chain = WebFilterChain { chainExchange ->
            chainExchange.response.statusCode = HttpStatus.CONFLICT
            chainExchange.response.headers.contentType = MediaType.APPLICATION_JSON
            chainExchange.response.writeWith(
                Mono.just(chainExchange.response.bufferFactory().wrap(body.toByteArray()))
            )
        }

        filter.filter(exchange, chain).block()

        assertEquals(HttpStatus.CONFLICT, exchange.response.statusCode)
        assertEquals(body, exchange.response.bodyAsString.block())
    }

    @Test
    fun `downstream connection failure maps to gateway 17801`() {
        val exchange = exchange("/v2/auth/me")
        val handler = GlobalExceptionHandler(
            responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*"),
            apiLogFactory = factory,
            apiStructuredLogger = ApiStructuredLogger()
        )

        handler.handle(exchange, ConnectException("connection refused")).block()

        val body = exchange.response.bodyAsString.block().orEmpty()
        assertEquals(HttpStatus.BAD_GATEWAY, exchange.response.statusCode)
        assertTrue(body.contains("\"code\":17801"))
    }

    @Test
    fun `gateway internal exception maps to 18801`() {
        val exchange = exchange("/v2/auth/me")
        val handler = GlobalExceptionHandler(
            responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*"),
            apiLogFactory = factory,
            apiStructuredLogger = ApiStructuredLogger()
        )

        handler.handle(exchange, IllegalStateException("gateway failure")).block()

        val body = exchange.response.bodyAsString.block().orEmpty()
        assertEquals(HttpStatus.INTERNAL_SERVER_ERROR, exchange.response.statusCode)
        assertTrue(body.contains("\"code\":18801"))
    }

    @Test
    fun `sensitive keys are masked`() {
        val masked = MaskingUtil.maskObject(
            mapOf(
                "password" to "password-raw",
                "passwordConfirm" to "password-confirm-raw",
                "accessToken" to "access-token-raw",
                "refreshToken" to "refresh-token-raw",
                "Authorization" to "Bearer authorization-raw",
                "Authenticate" to "Bearer authenticate-raw",
                "token" to "token-raw",
                "salt" to "salt-raw",
                "secret" to "secret-raw",
                "nested" to mapOf(
                    "PASSWORD" to "nested-password-raw",
                    "Authorization" to "nested-authorization-raw"
                )
            )
        ) as Map<*, *>

        listOf(
            "password",
            "passwordConfirm",
            "accessToken",
            "refreshToken",
            "Authorization",
            "Authenticate",
            "token",
            "salt",
            "secret"
        ).forEach { key -> assertEquals("****", masked[key]) }

        val nested = masked["nested"] as Map<*, *>
        assertEquals("****", nested["PASSWORD"])
        assertEquals("****", nested["Authorization"])
    }

    private fun exchange(path: String): MockServerWebExchange {
        return MockServerWebExchange.from(MockServerHttpRequest.get(path).build())
    }
}
