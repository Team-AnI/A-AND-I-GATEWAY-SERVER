package com.aandi.gateway.security

import com.aandi.gateway.common.HeaderNames
import org.springframework.core.Ordered
import org.springframework.security.core.Authentication
import org.springframework.security.core.context.ReactiveSecurityContextHolder
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono

@Component
class AuthenticatedPrincipalHeaderFilter : WebFilter, Ordered {

    override fun getOrder(): Int = Ordered.LOWEST_PRECEDENCE - 100

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        return ReactiveSecurityContextHolder.getContext()
            .mapNotNull { it.authentication }
            .filter(Authentication::isAuthenticated)
            .map { authentication -> withSanitizedPrincipalHeaders(exchange, authentication) }
            .defaultIfEmpty(withSanitizedPrincipalHeaders(exchange, null))
            .flatMap { sanitizedExchange -> chain.filter(sanitizedExchange) }
    }

    private fun withSanitizedPrincipalHeaders(
        exchange: ServerWebExchange,
        authentication: Authentication?
    ): ServerWebExchange {
        val request = exchange.request.mutate().headers { headers ->
            // Never trust caller-supplied identity headers.
            headers.remove(HeaderNames.USER_ID)
            headers.remove(HeaderNames.ROLES)
            if (authentication != null) {
                headers.set(HeaderNames.USER_ID, resolveSubject(authentication))
                headers.set(HeaderNames.ROLES, resolveRoles(authentication))
            }
        }.build()
        return exchange.mutate().request(request).build()
    }

    private fun resolveSubject(authentication: Authentication): String {
        if (authentication is JwtAuthenticationToken) {
            return authentication.token.subject
        }
        return authentication.name
    }

    private fun resolveRoles(authentication: Authentication): String {
        return authentication.authorities
            .asSequence()
            .map { it.authority }
            .joinToString(",")
    }
}
