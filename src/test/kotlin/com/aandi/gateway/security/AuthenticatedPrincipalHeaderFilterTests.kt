package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken
import org.springframework.security.core.authority.SimpleGrantedAuthority
import org.springframework.security.core.context.ReactiveSecurityContextHolder
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono
import kotlin.test.assertEquals
import kotlin.test.assertNull

class AuthenticatedPrincipalHeaderFilterTests {

    private val filter = AuthenticatedPrincipalHeaderFilter()

    @Test
    fun `adds user id and roles headers from authentication`() {
        val auth = UsernamePasswordAuthenticationToken(
            "user-123",
            "n/a",
            listOf(SimpleGrantedAuthority("ROLE_ADMIN"), SimpleGrantedAuthority("SCOPE_read"))
        )
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.get("/echo")
                .header("X-User-Id", "spoof-user")
                .header("X-Roles", "ROLE_SUPER_ADMIN")
                .build()
        )
        var userIdHeader: String? = null
        var rolesHeader: String? = null
        val chain = WebFilterChain { chainExchange ->
            userIdHeader = chainExchange.request.headers.getFirst("X-User-Id")
            rolesHeader = chainExchange.request.headers.getFirst("X-Roles")
            Mono.empty()
        }

        filter.filter(exchange, chain)
            .contextWrite(ReactiveSecurityContextHolder.withAuthentication(auth))
            .block()

        assertEquals("user-123", userIdHeader)
        assertEquals("ROLE_ADMIN,SCOPE_read", rolesHeader)
    }

    @Test
    fun `does not add headers when unauthenticated`() {
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.get("/echo")
                .header("X-User-Id", "spoof-user")
                .header("X-Roles", "ROLE_SUPER_ADMIN")
                .build()
        )
        var userIdHeader: String? = null
        var rolesHeader: String? = null
        val chain = WebFilterChain { chainExchange ->
            userIdHeader = chainExchange.request.headers.getFirst("X-User-Id")
            rolesHeader = chainExchange.request.headers.getFirst("X-Roles")
            Mono.empty()
        }

        filter.filter(exchange, chain).block()

        assertNull(userIdHeader)
        assertNull(rolesHeader)
    }
}
