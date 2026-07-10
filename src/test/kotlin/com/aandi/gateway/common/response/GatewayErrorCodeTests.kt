package com.aandi.gateway.common.response

import org.junit.jupiter.api.Test
import org.springframework.http.HttpStatus
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import tools.jackson.databind.ObjectMapper
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class GatewayErrorCodeTests {

    private val objectMapper = ObjectMapper()

    @Test
    fun `gateway error codes use five digits`() {
        GatewayErrorCode.entries.forEach { errorCode ->
            assertTrue(errorCode.code in 10000..99999)
        }
    }

    @Test
    fun `gateway error codes match gateway service domain`() {
        GatewayErrorCode.entries.forEach { errorCode ->
            assertEquals("Gateway", errorCode.service)
            assertEquals(1, errorCode.code.toString().first().digitToInt())
        }
    }

    @Test
    fun `gateway error code documentation matches enum contract`() {
        val document = Path.of(System.getProperty("user.dir"), "docs", "GATEWAY_ERROR_CODES.md")
        assertTrue(Files.isRegularFile(document), "Gateway error code document not found: $document")

        val documentedRows = Files.readAllLines(document).mapNotNull { line ->
            val columns = line.trim()
                .removePrefix("|")
                .removeSuffix("|")
                .split('|')
                .map(String::trim)
            val code = columns.getOrNull(0)?.toIntOrNull() ?: return@mapNotNull null
            val httpStatus = columns.getOrNull(2)?.toIntOrNull() ?: return@mapNotNull null
            val value = columns.getOrNull(3) ?: return@mapNotNull null
            val message = columns.getOrNull(4) ?: return@mapNotNull null
            val alert = columns.getOrNull(5) ?: return@mapNotNull null
            code to DocumentedErrorContract(httpStatus, value, message, alert)
        }
        val documented = documentedRows.toMap()
        val expectedRows = GatewayErrorCode.entries.map { errorCode ->
            errorCode.code to DocumentedErrorContract(
                httpStatus = errorCode.httpStatus.value(),
                value = errorCode.value,
                message = errorCode.message,
                alert = errorCode.alert
            )
        }
        val expected = expectedRows.toMap()

        assertEquals(documentedRows.size, documented.size, "Gateway error code document contains duplicate codes")
        assertEquals(expectedRows.size, expected.size, "GatewayErrorCode contains duplicate codes")
        assertEquals(expected, documented, "Gateway error code document is out of sync with GatewayErrorCode")
    }

    @Test
    fun `downstream service unavailable code exists`() {
        val errorCode = GatewayErrorCode.DOWNSTREAM_SERVICE_UNAVAILABLE

        assertEquals(17801, errorCode.code)
        assertEquals(HttpStatus.BAD_GATEWAY, errorCode.httpStatus)
        assertEquals("DOWNSTREAM_SERVICE_UNAVAILABLE", errorCode.value)
        assertEquals("active", errorCode.status)
        assertEquals("외부 시스템", errorCode.category)
        assertEquals("HIGH", errorCode.severity)
    }

    @Test
    fun `success response uses boolean success flag`() {
        val data = mapOf("invalidatedKeys" to 0)

        val response = GatewayResponse.success(data)

        assertTrue(response.success)
        assertEquals(data, response.data)
        assertEquals(null, response.error)
    }

    @Test
    fun `error response uses boolean failure flag and preserves payload`() {
        val errorCode = GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED

        val response = GatewayResponse.error(errorCode)

        assertFalse(response.success)
        assertEquals(null, response.data)
        assertEquals(errorCode.code, response.error?.code)
        assertEquals(errorCode.message, response.error?.message)
        assertEquals(errorCode.value, response.error?.value)
        assertEquals(errorCode.alert, response.error?.alert)
    }

    @Test
    fun `error response does not expose internal metadata`() {
        val json = objectMapper.writeValueAsString(GatewayResponse.error(GatewayErrorCode.DOWNSTREAM_SERVICE_UNAVAILABLE))

        assertTrue(json.contains("\"success\":false"))
        assertTrue(json.contains("\"data\":null"))
        assertTrue(json.contains("\"code\":17801"))
        assertTrue(json.contains("\"message\":"))
        assertTrue(json.contains("\"value\":\"DOWNSTREAM_SERVICE_UNAVAILABLE\""))
        assertTrue(json.contains("\"alert\":"))
        assertFalse(json.contains("\"status\":"))
        assertFalse(json.contains("\"service\":"))
        assertFalse(json.contains("\"category\":"))
        assertFalse(json.contains("\"severity\":"))
    }

    @Test
    fun `gateway response writer serializes failure flag and preserves status`() {
        val exchange = MockServerWebExchange.from(MockServerHttpRequest.get("/not-allowlisted").build())

        GatewayResponseWriter(objectMapper, "https://*")
            .writeError(exchange, GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED)
            .block()

        val body = exchange.response.bodyAsString.block().orEmpty()
        assertEquals(HttpStatus.NOT_FOUND, exchange.response.statusCode)
        assertTrue(body.contains("\"success\":false"))
        assertTrue(body.contains("\"data\":null"))
        assertTrue(body.contains("\"code\":15001"))
        assertTrue(body.contains("\"value\":\"ENDPOINT_NOT_ALLOWLISTED\""))
    }

    private data class DocumentedErrorContract(
        val httpStatus: Int,
        val value: String,
        val message: String,
        val alert: String
    )
}
