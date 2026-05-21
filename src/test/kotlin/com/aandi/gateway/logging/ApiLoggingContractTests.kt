package com.aandi.gateway.logging

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponseWriter
import org.junit.jupiter.api.Test
import org.springframework.cloud.gateway.support.ServerWebExchangeUtils
import org.springframework.http.HttpStatus
import org.springframework.http.MediaType
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.web.server.WebFilterChain
import org.springframework.web.server.ResponseStatusException
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
    fun `blank 401 403 and 404 response bodies use status aware fallback`() {
        assertGatewayError(
            logForResponse(HttpStatus.UNAUTHORIZED),
            GatewayErrorCode.AUTHENTICATION_FAILED
        )
        assertGatewayError(
            logForResponse(HttpStatus.FORBIDDEN),
            GatewayErrorCode.ACCESS_DENIED
        )
        assertGatewayError(
            logForResponse(HttpStatus.NOT_FOUND),
            GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED
        )
    }

    @Test
    fun `non json 401 403 and 404 response bodies use status aware fallback`() {
        assertGatewayError(
            logForResponse(HttpStatus.UNAUTHORIZED, "unauthorized"),
            GatewayErrorCode.AUTHENTICATION_FAILED
        )
        assertGatewayError(
            logForResponse(HttpStatus.FORBIDDEN, "forbidden"),
            GatewayErrorCode.ACCESS_DENIED
        )
        assertGatewayError(
            logForResponse(HttpStatus.NOT_FOUND, "not found"),
            GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED
        )
    }

    @Test
    fun `json errors missing structured error use status aware fallback`() {
        assertGatewayError(
            logForResponse(HttpStatus.UNAUTHORIZED, """{"success":false,"data":null}"""),
            GatewayErrorCode.AUTHENTICATION_FAILED
        )
        assertGatewayError(
            logForResponse(HttpStatus.FORBIDDEN, """{"success":false,"data":null}"""),
            GatewayErrorCode.ACCESS_DENIED
        )
        assertGatewayError(
            logForResponse(HttpStatus.NOT_FOUND, """{"success":false,"data":null}"""),
            GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED
        )
    }

    @Test
    fun `partial error object without code uses status aware fallback fields`() {
        assertGatewayError(
            logForResponse(HttpStatus.UNAUTHORIZED, """{"success":false,"error":{}}"""),
            GatewayErrorCode.AUTHENTICATION_FAILED
        )
        assertGatewayError(
            logForResponse(HttpStatus.FORBIDDEN, """{"success":false,"error":{}}"""),
            GatewayErrorCode.ACCESS_DENIED
        )
        assertGatewayError(
            logForResponse(HttpStatus.NOT_FOUND, """{"success":false,"error":{}}"""),
            GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED
        )
    }

    @Test
    fun `valid structured 401 and 403 error bodies are preserved`() {
        val unauthorized = logForResponse(
            HttpStatus.UNAUTHORIZED,
            """
                {
                  "success": false,
                  "data": null,
                  "error": {
                    "code": 21101,
                    "message": "access token is invalid",
                    "value": "ACCESS_TOKEN_INVALID",
                    "alert": "login required"
                  }
                }
            """.trimIndent()
        )
        val forbidden = logForResponse(
            HttpStatus.FORBIDDEN,
            """
                {
                  "success": false,
                  "data": null,
                  "error": {
                    "code": 12001,
                    "message": "custom denied",
                    "value": "ACCESS_DENIED",
                    "alert": "custom alert"
                  }
                }
            """.trimIndent()
        )

        assertEquals(21101, unauthorized.response.error?.code)
        assertEquals("ACCESS_TOKEN_INVALID", unauthorized.response.error?.value)
        assertEquals("access token is invalid", unauthorized.response.error?.message)
        assertEquals(GatewayErrorCode.ACCESS_DENIED.code, forbidden.response.error?.code)
        assertEquals("custom denied", forbidden.response.error?.message)
        assertEquals("custom alert", forbidden.response.error?.alert)
    }

    @Test
    fun `context response error is preserved over response body fallback`() {
        val exchange = exchange("/v2/lectures")
        exchange.response.statusCode = HttpStatus.UNAUTHORIZED
        val context = ApiLogContext.initialize(exchange)
        context.responseError = ApiLogError(
            code = GatewayErrorCode.AUTHENTICATION_FAILED.code,
            message = GatewayErrorCode.AUTHENTICATION_FAILED.message,
            value = GatewayErrorCode.AUTHENTICATION_FAILED.value,
            alert = GatewayErrorCode.AUTHENTICATION_FAILED.alert
        )
        context.responseBody = """{"success":false,"error":{"code":18801}}"""

        val log = factory.create(exchange, context, null)

        assertGatewayError(log, GatewayErrorCode.AUTHENTICATION_FAILED)
    }

    @Test
    fun `server errors without structured body remain internal error`() {
        assertGatewayError(
            logForResponse(HttpStatus.INTERNAL_SERVER_ERROR),
            GatewayErrorCode.INTERNAL_SERVER_ERROR
        )
        assertGatewayError(
            logForResponse(HttpStatus.INTERNAL_SERVER_ERROR, "server failure"),
            GatewayErrorCode.INTERNAL_SERVER_ERROR
        )
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
    fun `response status 401 maps to authentication failed`() {
        val exchange = exchange("/v2/lectures")
        val handler = GlobalExceptionHandler(
            responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*"),
            apiLogFactory = factory,
            apiStructuredLogger = ApiStructuredLogger()
        )

        handler.handle(exchange, ResponseStatusException(HttpStatus.UNAUTHORIZED, "unauthorized")).block()

        val body = exchange.response.bodyAsString.block().orEmpty()
        assertEquals(HttpStatus.UNAUTHORIZED, exchange.response.statusCode)
        assertTrue(body.contains("\"code\":11001"))
        assertTrue(body.contains("\"value\":\"AUTHENTICATION_FAILED\""))
        assertTrue(!body.contains("\"code\":18801"))
    }

    @Test
    fun `response status 403 maps to access denied`() {
        val exchange = exchange("/v2/problems/assignment-1/submissions/me")
        val handler = GlobalExceptionHandler(
            responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*"),
            apiLogFactory = factory,
            apiStructuredLogger = ApiStructuredLogger()
        )

        handler.handle(exchange, ResponseStatusException(HttpStatus.FORBIDDEN, "forbidden")).block()

        val body = exchange.response.bodyAsString.block().orEmpty()
        assertEquals(HttpStatus.FORBIDDEN, exchange.response.statusCode)
        assertTrue(body.contains("\"code\":12001"))
        assertTrue(body.contains("\"value\":\"ACCESS_DENIED\""))
        assertTrue(!body.contains("\"code\":18801"))
    }

    @Test
    fun `response status 404 maps to endpoint not allowlisted`() {
        val exchange = exchange("/v2/unknown")
        val handler = GlobalExceptionHandler(
            responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*"),
            apiLogFactory = factory,
            apiStructuredLogger = ApiStructuredLogger()
        )

        handler.handle(exchange, ResponseStatusException(HttpStatus.NOT_FOUND, "not found")).block()

        val body = exchange.response.bodyAsString.block().orEmpty()
        assertEquals(HttpStatus.NOT_FOUND, exchange.response.statusCode)
        assertTrue(body.contains("\"code\":15001"))
        assertTrue(body.contains("\"value\":\"ENDPOINT_NOT_ALLOWLISTED\""))
        assertTrue(!body.contains("\"code\":18801"))
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

    private fun logForResponse(status: HttpStatus, body: String = ""): ApiStructuredLog {
        val exchange = exchange("/v2/test")
        exchange.response.statusCode = status
        val context = ApiLogContext.initialize(exchange)
        context.responseBody = body
        return factory.create(exchange, context, null)
    }

    private fun assertGatewayError(log: ApiStructuredLog, errorCode: GatewayErrorCode) {
        val error = log.response.error
        assertNotNull(error)
        assertEquals(errorCode.code, error.code)
        assertEquals(errorCode.value, error.value)
        assertEquals(errorCode.message, error.message)
        assertEquals(errorCode.alert, error.alert)
        if (errorCode != GatewayErrorCode.INTERNAL_SERVER_ERROR) {
            assertTrue(error.code != GatewayErrorCode.INTERNAL_SERVER_ERROR.code)
            assertTrue(error.value != GatewayErrorCode.INTERNAL_SERVER_ERROR.value)
        }
    }
}
