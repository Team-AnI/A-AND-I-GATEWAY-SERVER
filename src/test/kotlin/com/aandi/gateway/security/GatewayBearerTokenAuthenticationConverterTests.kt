package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.http.HttpHeaders
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.security.oauth2.server.resource.authentication.BearerTokenAuthenticationToken
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class GatewayBearerTokenAuthenticationConverterTests {

    private val converter = GatewayBearerTokenAuthenticationConverter()

    @Test
    fun `authorization bearer header is accepted`() {
        val exchange = exchange(
            HttpHeaders.AUTHORIZATION to "Bearer access-token"
        )

        val authentication = converter.convert(exchange).block()

        assertNotNull(authentication)
        assertEquals("access-token", (authentication as BearerTokenAuthenticationToken).token)
    }

    @Test
    fun `authenticate bearer header is accepted`() {
        val exchange = exchange(
            "Authenticate" to "Bearer access-token"
        )

        val authentication = converter.convert(exchange).block()

        assertNotNull(authentication)
        assertEquals("access-token", (authentication as BearerTokenAuthenticationToken).token)
    }

    @Test
    fun `authenticate header without bearer prefix is rejected`() {
        val exchange = exchange(
            "Authenticate" to "access-token"
        )

        val authentication = converter.convert(exchange).block()

        assertNull(authentication)
    }

    private fun exchange(vararg headers: Pair<String, String>) =
        MockServerWebExchange.from(
            MockServerHttpRequest.get("/v2/me").apply {
                headers.forEach { (name, value) -> header(name, value) }
            }.build(),
        )
}
