package com.aandi.gateway.common.response

import com.aandi.gateway.logging.ApiLogContext
import org.springframework.beans.factory.annotation.Value
import org.springframework.http.MediaType
import org.springframework.stereotype.Component
import org.springframework.util.PatternMatchUtils
import org.springframework.web.server.ServerWebExchange
import reactor.core.publisher.Mono
import java.time.OffsetDateTime
import java.time.ZoneId
import tools.jackson.databind.ObjectMapper

@Component
class GatewayResponseWriter(
    private val objectMapper: ObjectMapper,
    @Value("\${CORS_ALLOWED_ORIGIN_PATTERNS:https://*}") allowedOriginPatternsRaw: String
) {
    private val allowedOriginPatterns = allowedOriginPatternsRaw
        .split(",")
        .map { it.trim() }
        .filter { it.isNotEmpty() }
        .ifEmpty { listOf("https://*") }

    fun writeError(
        exchange: ServerWebExchange,
        errorCode: GatewayErrorCode,
        includeCorsHeaders: Boolean = true
    ): Mono<Void> {
        val context = ApiLogContext.get(exchange)
        context.markFailure("${errorCode.value}: ${errorCode.message}")
        context.responseError = com.aandi.gateway.logging.ApiLogError(
            code = errorCode.code,
            message = errorCode.message,
            value = errorCode.value,
            alert = errorCode.alert
        )

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
        if (origin.isBlank() || !isAllowedOrigin(origin)) {
            return
        }

        val headers = exchange.response.headers
        headers.set("Access-Control-Allow-Origin", origin)
        if (headers.getFirst("Vary") == null) {
            headers.add("Vary", "Origin")
        }
    }

    private fun isAllowedOrigin(origin: String): Boolean {
        return allowedOriginPatterns.any { pattern -> PatternMatchUtils.simpleMatch(pattern, origin) }
    }

    companion object {
        private val KOREA_ZONE_ID: ZoneId = ZoneId.of("Asia/Seoul")
    }
}
