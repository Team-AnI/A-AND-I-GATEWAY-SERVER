package com.aandi.gateway.common.response

import org.junit.jupiter.api.Test
import org.springframework.http.HttpStatus
import tools.jackson.databind.ObjectMapper
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
    fun `error response does not expose internal metadata`() {
        val json = objectMapper.writeValueAsString(GatewayResponse.error(GatewayErrorCode.DOWNSTREAM_SERVICE_UNAVAILABLE))

        assertTrue(json.contains("\"code\":17801"))
        assertTrue(json.contains("\"message\":"))
        assertTrue(json.contains("\"value\":\"DOWNSTREAM_SERVICE_UNAVAILABLE\""))
        assertTrue(json.contains("\"alert\":"))
        assertFalse(json.contains("\"status\":"))
        assertFalse(json.contains("\"service\":"))
        assertFalse(json.contains("\"category\":"))
        assertFalse(json.contains("\"severity\":"))
    }
}
