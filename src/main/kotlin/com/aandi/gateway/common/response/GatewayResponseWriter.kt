package com.aandi.gateway.common.response

import com.aandi.gateway.logging.ApiLogContext
import org.springframework.http.MediaType
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import reactor.core.publisher.Mono
import java.time.OffsetDateTime
import java.time.ZoneId
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
        val context = ApiLogContext.get(exchange)
        context.markFailure("${errorCode.value}: ${errorCode.message}")

        val response = exchange.response
        response.statusCode = errorCode.httpStatus
        if (includeCorsHeaders) {
            applyCorsHeaders(exchange)
        }
        response.headers.contentType = MediaType.APPLICATION_JSON
        val bodyBytes = objectMapper.writeValueAsBytes(GatewayResponse.error(errorCode))
        context.responseBody = bodyBytes.toString(Charsets.UTF_8)
        context.responseTimestamp = OffsetDateTime.now(KOREA_ZONE_ID).toString()
        return response.writeWith(Mono.just(response.bufferFactory().wrap(bodyBytes)))
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

    companion object {
        private val KOREA_ZONE_ID: ZoneId = ZoneId.of("Asia/Seoul")
    }
}
