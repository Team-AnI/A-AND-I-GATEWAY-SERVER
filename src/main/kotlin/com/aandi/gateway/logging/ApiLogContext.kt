package com.aandi.gateway.logging

import org.springframework.web.server.ServerWebExchange
import java.time.Instant
import java.util.UUID

data class ApiLogContext(
    val traceId: String,
    val requestId: String,
    val startedAt: Instant,
    var requestBody: String = "",
    var responseBody: String = "",
    var failureMessage: String? = null,
    var responseTimestamp: String? = null
) {
    fun markFailure(message: String?) {
        if (!message.isNullOrBlank()) {
            failureMessage = message
        }
    }

    companion object {
        private const val ATTRIBUTE_NAME = "aandi.api-log-context"

        fun initialize(exchange: ServerWebExchange): ApiLogContext {
            val existing = exchange.getAttribute<ApiLogContext>(ATTRIBUTE_NAME)
            if (existing != null) {
                return existing
            }

            val requestId = exchange.request.headers.getFirst(REQUEST_ID_HEADER)
                ?.takeIf { it.isNotBlank() }
                ?: UUID.randomUUID().toString()

            val context = ApiLogContext(
                traceId = UUID.randomUUID().toString(),
                requestId = requestId,
                startedAt = Instant.now()
            )
            exchange.attributes[ATTRIBUTE_NAME] = context
            exchange.response.headers.add(REQUEST_ID_HEADER, context.requestId)
            return context
        }

        fun get(exchange: ServerWebExchange): ApiLogContext {
            return exchange.getAttribute<ApiLogContext>(ATTRIBUTE_NAME) ?: initialize(exchange)
        }

        private const val REQUEST_ID_HEADER = "X-Request-Id"
    }
}
