package com.aandi.gateway.security

import org.springframework.http.HttpMethod
import org.springframework.http.HttpStatus
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken
import org.springframework.security.core.GrantedAuthority
import org.springframework.security.core.authority.SimpleGrantedAuthority
import org.springframework.security.core.context.ReactiveSecurityContextHolder
import org.springframework.security.web.server.WebFilterChainProxy
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono
import java.net.URI
import kotlin.test.assertEquals

internal class SecurityChainProbe(
    private val springSecurity: WebFilterChainProxy
) {
    private val authenticatedPrincipals = listOf(
        PrincipalCase("scope-only", listOf(SimpleGrantedAuthority("SCOPE_read")), role = null)
    ) + UserRole.entries.map { role ->
        PrincipalCase(
            role.name,
            listOf(SimpleGrantedAuthority("ROLE_${role.name}")),
            role
        )
    }

    fun assertAccess(
        method: HttpMethod,
        path: String,
        requirement: AccessRequirement,
        context: String = "$method $path"
    ) {
        when (requirement) {
            AccessRequirement.PermitAll -> {
                assertEquals(HttpStatus.NO_CONTENT, exchange(method, path), "$context anonymous")
                authenticatedPrincipals.forEach { principal ->
                    assertEquals(
                        HttpStatus.NO_CONTENT,
                        exchange(method, path, principal.authorities),
                        "$context ${principal.label}"
                    )
                }
            }

            AccessRequirement.Authenticated -> {
                assertEquals(HttpStatus.UNAUTHORIZED, exchange(method, path), "$context anonymous")
                authenticatedPrincipals.forEach { principal ->
                    assertEquals(
                        HttpStatus.NO_CONTENT,
                        exchange(method, path, principal.authorities),
                        "$context ${principal.label}"
                    )
                }
            }

            is AccessRequirement.AnyRole -> {
                assertEquals(HttpStatus.UNAUTHORIZED, exchange(method, path), "$context anonymous")
                authenticatedPrincipals.forEach { principal ->
                    val expected = if (principal.role != null && principal.role in requirement.roles) {
                        HttpStatus.NO_CONTENT
                    } else {
                        HttpStatus.FORBIDDEN
                    }
                    assertEquals(
                        expected,
                        exchange(method, path, principal.authorities),
                        "$context ${principal.label}"
                    )
                }
            }
        }
    }

    fun exchange(
        method: HttpMethod,
        path: String,
        authorities: Collection<GrantedAuthority>? = null
    ): HttpStatus {
        val request = MockServerHttpRequest.method(method, URI.create("http://localhost$path")).build()
        val exchange = MockServerWebExchange.from(request)
        val result = springSecurity.filter(exchange, terminalChain).let { filtered ->
            if (authorities == null) {
                filtered
            } else {
                val authentication = UsernamePasswordAuthenticationToken.authenticated(
                    "principal",
                    "credentials",
                    authorities
                )
                filtered.contextWrite(ReactiveSecurityContextHolder.withAuthentication(authentication))
            }
        }

        result.block()
        val status = requireNotNull(exchange.response.statusCode) {
            "Security chain did not complete the response for $method $path"
        }
        return HttpStatus.valueOf(status.value())
    }

    private data class PrincipalCase(
        val label: String,
        val authorities: List<GrantedAuthority>,
        val role: UserRole?
    )

    private companion object {
        val terminalChain = WebFilterChain { exchange: ServerWebExchange ->
            exchange.response.statusCode = HttpStatus.NO_CONTENT
            Mono.empty()
        }
    }
}
