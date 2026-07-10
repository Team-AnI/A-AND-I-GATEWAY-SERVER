package com.aandi.gateway.security

import com.aandi.gateway.common.response.GatewayResponseWriter
import org.junit.jupiter.api.Test
import org.springframework.http.HttpHeaders
import org.springframework.http.HttpStatus
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono
import tools.jackson.databind.ObjectMapper
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class GatewayHostPolicyTests {

    private val responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*")

    @Test
    fun `configured hostname remains case insensitive`() {
        val result = evaluateHost(
            host = "api.aandiclub.com",
            allowedHosts = setOf("API.AANDICLUB.COM"),
            allowPrivateIpHost = false
        )

        assertTrue(result.chainInvoked)
        assertNull(result.status)
    }

    @Test
    fun `private and loopback IP literals can bypass the configured host allowlist`() {
        listOf("10.0.0.1", "172.16.0.1", "192.168.0.1", "127.0.0.1", "[::1]:8080").forEach { host ->
            val result = evaluateHost(host)

            assertTrue(result.chainInvoked, host)
            assertNull(result.status, host)
        }
    }

    @Test
    fun `hostname resolving to loopback does not bypass the configured host allowlist`() {
        val result = evaluateHost("localhost")

        assertFalse(result.chainInvoked)
        assertEquals(HttpStatus.FORBIDDEN, result.status)
    }

    @Test
    fun `public IP literal does not bypass the configured host allowlist`() {
        val result = evaluateHost("8.8.8.8")

        assertFalse(result.chainInvoked)
        assertEquals(HttpStatus.FORBIDDEN, result.status)
    }

    @Test
    fun `private IP bypass can be disabled`() {
        val result = evaluateHost(host = "10.0.0.1", allowPrivateIpHost = false)

        assertFalse(result.chainInvoked)
        assertEquals(HttpStatus.FORBIDDEN, result.status)
    }

    @Test
    fun `empty configured host allowlist bypasses host validation`() {
        val result = evaluateHost(host = "unlisted.example", allowedHosts = emptySet())

        assertTrue(result.chainInvoked)
        assertNull(result.status)
    }

    @Test
    fun `literal parser preserves loopback and site local address boundaries`() {
        listOf(
            "10.0.0.0",
            "10.255.255.255",
            "172.16.0.0",
            "172.31.255.255",
            "192.168.0.0",
            "192.168.255.255",
            "127.0.0.1",
            "127.255.255.255",
            "::1",
            "[::1]",
            "fec0::1",
            "feff::1",
            "::ffff:192.168.1.1",
            "::ffff:127.0.0.1"
        ).forEach { host ->
            assertTrue(isLoopbackOrSiteLocalIpLiteral(host), host)
        }
    }

    @Test
    fun `literal parser rejects public special malformed and hostname inputs`() {
        listOf(
            "9.255.255.255",
            "11.0.0.0",
            "172.15.255.255",
            "172.32.0.0",
            "192.167.255.255",
            "192.169.0.0",
            "169.254.1.1",
            "0.0.0.0",
            "224.0.0.1",
            "8.8.8.8",
            "::",
            "febf::1",
            "fc00::1",
            "fd00::1",
            "fe80::1",
            "2001:db8::1",
            "fe80::1%lo0",
            "[fec0::1%25lo0]",
            "",
            "localhost",
            "gateway",
            "private.internal",
            "127.1",
            "2130706433",
            "256.1.1.1",
            "1.2.3",
            "[::1]:8080",
            " 10.0.0.1"
        ).forEach { host ->
            assertFalse(isLoopbackOrSiteLocalIpLiteral(host), host)
        }
    }

    private fun evaluateHost(
        host: String,
        allowedHosts: Set<String> = setOf("api.aandiclub.com"),
        allowPrivateIpHost: Boolean = true
    ): HostPolicyResult {
        val filter = GatewayRequestPolicyFilter(
            policy = SecurityPolicyProperties(
                allowedHosts = allowedHosts,
                allowPrivateIpHost = allowPrivateIpHost,
                enforceMethodPathAllowlist = false,
                enforceJsonContentType = false
            ),
            responseWriter = responseWriter
        )
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.get("/v2/ping")
                .header(HttpHeaders.HOST, host)
                .build()
        )
        var chainInvoked = false
        val chain = WebFilterChain {
            chainInvoked = true
            Mono.empty()
        }

        filter.filter(exchange, chain).block()

        return HostPolicyResult(chainInvoked, exchange.response.statusCode)
    }

    private data class HostPolicyResult(
        val chainInvoked: Boolean,
        val status: org.springframework.http.HttpStatusCode?
    )
}
