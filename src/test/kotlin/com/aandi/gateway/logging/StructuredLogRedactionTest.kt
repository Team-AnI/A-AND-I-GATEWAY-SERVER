package com.aandi.gateway.logging

import ch.qos.logback.classic.Logger
import ch.qos.logback.classic.spi.ILoggingEvent
import ch.qos.logback.core.read.ListAppender
import org.junit.jupiter.api.Test
import org.slf4j.LoggerFactory
import org.springframework.http.HttpHeaders
import org.springframework.http.HttpStatus
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import tools.jackson.databind.ObjectMapper
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class StructuredLogRedactionTest {

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
    fun `structured log contains request fields and redacts sensitive values in captured output`() {
        val request = MockServerHttpRequest.post(
            "/v1/auth/login?refreshToken=query-refresh-token&username=demo-user"
        )
            .header(ApiLogContext.TRACE_ID_HEADER, "trace-log-test")
            .header(ApiLogContext.REQUEST_ID_HEADER, "request-log-test")
            .header(HttpHeaders.AUTHORIZATION, "Bearer jwt.raw.value")
            .build()
        val exchange = MockServerWebExchange.from(request)
        exchange.response.statusCode = HttpStatus.OK
        val context = ApiLogContext.initialize(exchange)
        context.requestBody = """
            {
              "username": "demo-user",
              "password": "password-raw",
              "refreshToken": "refresh-token-raw",
              "discordToken": "discord-token-raw"
            }
        """.trimIndent()
        context.responseBody = """{"success":true,"data":{"ok":true},"error":null}"""

        val payload = factory.create(exchange, context, null)
        val json = captureStructuredJson(payload)

        assertTrue(json.contains(""""traceId":"trace-log-test""""))
        assertTrue(json.contains(""""requestId":"request-log-test""""))
        assertTrue(json.contains(""""method":"POST""""))
        assertTrue(json.contains(""""path":"/v1/auth/login""""))
        assertTrue(json.contains(""""statusCode":200"""))
        assertTrue(json.contains(""""latencyMs":"""))
        assertTrue(json.contains(""""Authenticate":"Bearer ****""""))
        assertTrue(json.contains(""""password":"****""""))
        assertTrue(json.contains(""""refreshToken":"****""""))
        assertTrue(json.contains(""""discordToken":"****""""))
        assertTrue(json.contains(""""username":"dem******""""))

        listOf(
            "jwt.raw.value",
            "password-raw",
            "refresh-token-raw",
            "discord-token-raw",
            "query-refresh-token"
        ).forEach { raw ->
            assertTrue(!json.contains(raw), "structured log leaked $raw: $json")
        }
    }

    @Test
    fun `masking utility redacts discord token and webhook shaped keys`() {
        val masked = MaskingUtil.maskObject(
            mapOf(
                "discordToken" to "discord-token-raw",
                "DISCORD_BOT_TOKEN" to "discord-bot-token-raw",
                "webhookUrl" to "webhook-url-raw"
            )
        ) as Map<*, *>

        assertEquals("****", masked["discordToken"])
        assertEquals("****", masked["DISCORD_BOT_TOKEN"])
        assertEquals("****", masked["webhookUrl"])
    }

    private fun captureStructuredJson(payload: ApiStructuredLog): String {
        val logger = LoggerFactory.getLogger("AANDI_API_LOG") as Logger
        val appender = ListAppender<ILoggingEvent>()
        appender.start()
        logger.addAppender(appender)
        try {
            ApiStructuredLogger().log(payload)
            val event = appender.list.singleOrNull()
            assertNotNull(event)
            return StructuredJsonLogLayout().doLayout(event)
        } finally {
            logger.detachAppender(appender)
        }
    }
}
