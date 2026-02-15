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
        return ReactiveSecurityContextHolder.getContext()
            .mapNotNull { it.authentication }
            .filter(Authentication::isAuthenticated)
            .flatMap { authentication ->
                tokenContextResolver.resolve(authentication)
                    .flatMap { result ->
                        chain.filter(withContextHeaders(exchange, result))
                    }
            }
            .switchIfEmpty(chain.filter(withContextHeaders(exchange, null)))
    }

    private fun withContextHeaders(
        exchange: ServerWebExchange,
        resolution: TokenContextResolution?
    ): ServerWebExchange {
        val request = exchange.request.mutate().headers { headers ->
            headers.remove(HeaderNames.AUTH_CONTEXT)
            headers.remove(HeaderNames.AUTH_CONTEXT_CACHE)
            if (resolution != null) {
                headers.set(HeaderNames.AUTH_CONTEXT, resolution.payloadJson)
                headers.set(HeaderNames.AUTH_CONTEXT_CACHE, toCacheStatus(resolution))
            }
        }.build()
        return exchange.mutate().request(request).build()
    }

    private fun toCacheStatus(resolution: TokenContextResolution): String {
        if (resolution.cacheHit) {
            return "HIT"
        }
        return "MISS"
    }
}
