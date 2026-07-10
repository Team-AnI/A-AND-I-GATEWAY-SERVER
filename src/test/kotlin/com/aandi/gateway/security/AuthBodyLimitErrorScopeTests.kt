package com.aandi.gateway.security

import com.aandi.gateway.common.response.GatewayResponseWriter
import org.junit.jupiter.api.Test
import org.springframework.core.io.buffer.DataBufferLimitException
import org.springframework.http.MediaType
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.util.unit.DataSize
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono
import reactor.test.StepVerifier
import tools.jackson.databind.ObjectMapper
import kotlin.test.assertNull

class AuthBodyLimitErrorScopeTests {

    private val policy = SecurityPolicyProperties(maxRequestBodySize = DataSize.ofKilobytes(1))
    private val bodyCache = AuthRequestBodyCache(policy)
    private val responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*")

    @Test
    fun `rate limit filter does not convert a downstream buffer error to body too large`() {
        assertDownstreamErrorPropagates(
            AuthRateLimitFilter(RateLimitProperties(), responseWriter, bodyCache)
        )
    }

    @Test
    fun `validation filter does not convert a downstream buffer error to body too large`() {
        assertDownstreamErrorPropagates(
            AuthRequestValidationFilter(JwtPolicyProperties(), policy, responseWriter, bodyCache)
        )
    }

    private fun assertDownstreamErrorPropagates(filter: WebFilter) {
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.post("/v1/auth/login")
                .contentType(MediaType.APPLICATION_JSON)
                .body("""{"username":"user","password":"password"}""")
        )
        val downstreamError = DataBufferLimitException("downstream failure")
        val chain = WebFilterChain { Mono.error(downstreamError) }

        StepVerifier.create(filter.filter(exchange, chain))
            .expectErrorMatches { error -> error === downstreamError }
            .verify()
        assertNull(exchange.response.statusCode)
    }
}
