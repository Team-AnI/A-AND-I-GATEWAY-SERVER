package com.aandi.gateway.cache

import org.junit.jupiter.api.Test
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken
import org.springframework.security.core.context.ReactiveSecurityContextHolder
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono
import kotlin.test.assertTrue
import kotlin.test.assertNull

class TokenContextHeaderFilterTests {

    @Test
    fun `authenticated request warms cache and strips context headers`() {
        var resolverCalled = false
        val filter = TokenContextHeaderFilter(
            object : TokenContextResolver {
                override fun resolve(authentication: org.springframework.security.core.Authentication): Mono<TokenContextResolution> {
                    resolverCalled = true
                    return Mono.just(TokenContextResolution("""{"subject":"user-1"}""", true))
                }
            }
        )
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.get("/api/test")
                .header("X-Auth-Context", "spoof")
                .header("X-Auth-Context-Cache", "spoof")
                .build()
        )
        val auth = UsernamePasswordAuthenticationToken("user-1", "n/a", emptyList())

        var contextHeader: String? = null
        var cacheHeader: String? = null
        val chain = WebFilterChain { chainExchange ->
            contextHeader = chainExchange.request.headers.getFirst("X-Auth-Context")
            cacheHeader = chainExchange.request.headers.getFirst("X-Auth-Context-Cache")
            Mono.empty()
        }

        filter.filter(exchange, chain)
            .contextWrite(ReactiveSecurityContextHolder.withAuthentication(auth))
            .block()

        assertTrue(resolverCalled)
        assertNull(contextHeader)
        assertNull(cacheHeader)
    }

    @Test
    fun `unauthenticated request strips spoofed cache headers`() {
        val filter = TokenContextHeaderFilter(
            object : TokenContextResolver {
                override fun resolve(authentication: org.springframework.security.core.Authentication): Mono<TokenContextResolution> {
                    return Mono.error(IllegalStateException("should not be called"))
                }
            }
        )
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.get("/api/test")
                .header("X-Auth-Context", "spoof")
                .header("X-Auth-Context-Cache", "spoof")
                .build()
        )

        var contextHeader: String? = null
        var cacheHeader: String? = null
        val chain = WebFilterChain { chainExchange ->
            contextHeader = chainExchange.request.headers.getFirst("X-Auth-Context")
            cacheHeader = chainExchange.request.headers.getFirst("X-Auth-Context-Cache")
            Mono.empty()
        }

        filter.filter(exchange, chain).block()

        assertNull(contextHeader)
        assertNull(cacheHeader)
    }
}
