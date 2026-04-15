package com.aandi.gateway.common.response

import com.aandi.gateway.logging.ApiLogContext
import org.springframework.http.MediaType
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import reactor.core.publisher.Mono
import tools.jackson.databind.ObjectMapper

@Component
class GatewayResponseWriter(
    private val objectMapper: ObjectMapper
) {
    fun writeError(
        exchange: ServerWebExchange,
        errorCode: GatewayErrorCode,
        includeCorsHeaders: Boolean = true
    ): Mono<Void> {
        ApiLogContext.get(exchange).markFailure("${errorCode.value}: ${errorCode.message}")

        val response = exchange.response
        response.statusCode = errorCode.httpStatus
        if (includeCorsHeaders) {
            applyCorsHeaders(exchange)
        }
        response.headers.contentType = MediaType.APPLICATION_JSON
        val body = objectMapper.writeValueAsBytes(GatewayResponse.error(errorCode))
        return response.writeWith(Mono.just(response.bufferFactory().wrap(body)))
    }

    private fun applyCorsHeaders(exchange: ServerWebExchange) {
        val origin = exchange.request.headers.origin?.trim().orEmpty()
        if (origin.isBlank()) {
            return
        }

        val headers = exchange.response.headers
        headers.set("Access-Control-Allow-Origin", origin)
        if (headers.getFirst("Vary") == null) {
            headers.add("Vary", "Origin")
        }
    }
}
