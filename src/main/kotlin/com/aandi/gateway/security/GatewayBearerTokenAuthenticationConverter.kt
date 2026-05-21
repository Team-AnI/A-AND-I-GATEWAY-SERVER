package com.aandi.gateway.security

import org.springframework.http.HttpHeaders
import org.springframework.security.authentication.AbstractAuthenticationToken
import org.springframework.security.core.Authentication
import org.springframework.security.oauth2.server.resource.authentication.BearerTokenAuthenticationToken
import org.springframework.security.web.server.authentication.ServerAuthenticationConverter
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import reactor.core.publisher.Mono

@Component
class GatewayBearerTokenAuthenticationConverter : ServerAuthenticationConverter {

    override fun convert(exchange: ServerWebExchange): Mono<Authentication> {
        val headers = exchange.request.headers
        val token = resolveBearerToken(headers.getFirst(HttpHeaders.AUTHORIZATION))
            ?: resolveBearerToken(headers.getFirst(AUTHENTICATE_HEADER))
            ?: return Mono.empty()

        return Mono.just(BearerTokenAuthenticationToken(token))
    }

    private fun resolveBearerToken(headerValue: String?): String? {
        val raw = headerValue?.trim().orEmpty()
        if (!raw.startsWith(BEARER_PREFIX, ignoreCase = true)) {
            return null
        }
        return raw.substring(BEARER_PREFIX.length).trim().takeIf { it.isNotBlank() }
    }

    companion object {
        private const val BEARER_PREFIX = "Bearer "
        private const val AUTHENTICATE_HEADER = "Authenticate"
    }
}
