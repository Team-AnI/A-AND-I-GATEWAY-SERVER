package com.aandi.gateway.security

import org.springframework.beans.factory.annotation.Value
import org.springframework.core.Ordered
import org.springframework.stereotype.Component
import org.springframework.util.PatternMatchUtils
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono

@Component
class CorsResponseHeaderFilter(
    @Value("\${CORS_ALLOWED_ORIGIN_PATTERNS:https://*}") allowedOriginPatternsRaw: String
) : WebFilter, Ordered {

    private val allowedOriginPatterns = allowedOriginPatternsRaw
        .split(",")
        .map { it.trim() }
        .filter { it.isNotEmpty() }
        .ifEmpty { listOf("https://*") }

    override fun getOrder(): Int = Ordered.HIGHEST_PRECEDENCE

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        val origin = exchange.request.headers.origin?.trim().orEmpty()
        if (origin.isBlank() || !isAllowedOrigin(origin)) {
            return chain.filter(exchange)
        }

        exchange.response.beforeCommit {
            val headers = exchange.response.headers
            if (headers.getFirst("Access-Control-Allow-Origin") == null) {
                headers.set("Access-Control-Allow-Origin", origin)
            }
            if (headers.getFirst("Vary") == null) {
                headers.add("Vary", "Origin")
            }
            Mono.empty()
        }

        return chain.filter(exchange)
    }

    private fun isAllowedOrigin(origin: String): Boolean {
        return allowedOriginPatterns.any { pattern -> PatternMatchUtils.simpleMatch(pattern, origin) }
    }
}
