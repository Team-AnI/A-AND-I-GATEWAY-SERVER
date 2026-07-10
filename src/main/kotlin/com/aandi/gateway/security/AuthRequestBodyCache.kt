package com.aandi.gateway.security

import org.springframework.core.io.buffer.DataBuffer
import org.springframework.core.io.buffer.DataBufferLimitException
import org.springframework.core.io.buffer.DataBufferUtils
import org.springframework.http.server.reactive.ServerHttpRequestDecorator
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import reactor.core.publisher.Flux
import reactor.core.publisher.Mono

@Component
class AuthRequestBodyCache(
    policy: SecurityPolicyProperties
) {
    private val maxBytes = policy.maxRequestBodySize.toBytes().let { configuredBytes ->
        require(configuredBytes in 1..Int.MAX_VALUE.toLong()) {
            "app.security.policy.max-request-body-size must be between 1 byte and ${Int.MAX_VALUE} bytes"
        }
        configuredBytes.toInt()
    }

    internal fun read(exchange: ServerWebExchange): Mono<AuthRequestBodyReadResult> {
        val cached = exchange.getAttribute<ByteArray>(ATTRIBUTE_NAME)
        if (cached != null) {
            return Mono.just(AuthRequestBodyReadResult.Available(cached))
        }

        return DataBufferUtils.join(exchange.request.body, maxBytes)
            .map { buffer -> readAndRelease(buffer) }
            .switchIfEmpty(Mono.fromSupplier { ByteArray(0) })
            .doOnNext { bytes -> exchange.attributes[ATTRIBUTE_NAME] = bytes }
            .map<AuthRequestBodyReadResult>(AuthRequestBodyReadResult::Available)
            .onErrorReturn(DataBufferLimitException::class.java, AuthRequestBodyReadResult.TooLarge)
    }

    internal fun decorate(exchange: ServerWebExchange, bytes: ByteArray): ServerWebExchange {
        val request = object : ServerHttpRequestDecorator(exchange.request) {
            override fun getBody(): Flux<DataBuffer> {
                if (bytes.isEmpty()) {
                    return Flux.empty()
                }
                return Flux.defer {
                    Mono.just(exchange.response.bufferFactory().wrap(bytes))
                }
            }
        }
        return exchange.mutate().request(request).build()
    }

    private fun readAndRelease(buffer: DataBuffer): ByteArray {
        return try {
            ByteArray(buffer.readableByteCount()).also(buffer::read)
        } finally {
            DataBufferUtils.release(buffer)
        }
    }

    private companion object {
        private const val ATTRIBUTE_NAME = "aandi.auth-request-body"
    }
}

internal sealed interface AuthRequestBodyReadResult {
    data class Available(val bytes: ByteArray) : AuthRequestBodyReadResult

    data object TooLarge : AuthRequestBodyReadResult
}
