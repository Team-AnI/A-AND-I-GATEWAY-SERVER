package com.aandi.gateway.cache

import com.aandi.gateway.common.HeaderNames
import org.springframework.core.Ordered
import org.springframework.security.core.Authentication
import org.springframework.security.core.context.ReactiveSecurityContextHolder
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono

@Component
class TokenContextHeaderFilter(
    private val tokenContextResolver: TokenContextResolver
) : WebFilter, Ordered {

    override fun getOrder(): Int = Ordered.LOWEST_PRECEDENCE - 200

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        val sanitizedExchange = sanitizeContextHeaders(exchange)
        return ReactiveSecurityContextHolder.getContext()
            .mapNotNull { it.authentication }
            .filter(Authentication::isAuthenticated)
            .flatMap { authentication ->
                tokenContextResolver.resolve(authentication)
                    .thenReturn(sanitizedExchange)
            }
            .defaultIfEmpty(sanitizedExchange)
            .flatMap { contextExchange -> chain.filter(contextExchange) }
    }

    private fun sanitizeContextHeaders(exchange: ServerWebExchange): ServerWebExchange {
        val request = exchange.request.mutate().headers { headers ->
            headers.remove(HeaderNames.AUTH_CONTEXT)
            headers.remove(HeaderNames.AUTH_CONTEXT_CACHE)
        }.build()
        return exchange.mutate().request(request).build()
    }
}
