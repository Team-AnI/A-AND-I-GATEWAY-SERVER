package com.aandi.gateway.logging

import org.reactivestreams.Publisher
import org.springframework.core.Ordered
import org.springframework.core.io.buffer.DataBuffer
import org.springframework.core.io.buffer.DataBufferUtils
import org.springframework.http.HttpMethod
import org.springframework.http.MediaType
import org.springframework.http.server.reactive.ServerHttpRequestDecorator
import org.springframework.http.server.reactive.ServerHttpResponseDecorator
import org.springframework.security.core.Authentication
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Flux
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets
import java.security.Principal
import java.util.Optional

@Component
class RequestResponseLoggingFilter(
    private val apiLogFactory: ApiLogFactory,
    private val apiStructuredLogger: ApiStructuredLogger
) : WebFilter, Ordered {

    override fun getOrder(): Int = Ordered.HIGHEST_PRECEDENCE + 5

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        val context = ApiLogContext.initialize(exchange)
        return cacheRequestBody(exchange, context)
            .flatMap { requestExchange ->
                val decoratedResponse = LoggingResponseDecorator(requestExchange, context)
                val decoratedExchange = ApiLogContext.attach(
                    requestExchange.mutate().response(decoratedResponse).build(),
                    context
                )
                chain.filter(decoratedExchange)
                    .then(resolveAuthentication(decoratedExchange))
                    .doOnNext { authentication ->
                        apiStructuredLogger.log(apiLogFactory.create(decoratedExchange, context, authentication.orElse(null)))
                    }
                    .then()
            }
    }

    private fun cacheRequestBody(exchange: ServerWebExchange, context: ApiLogContext): Mono<ServerWebExchange> {
        if (!shouldCaptureRequestBody(exchange)) {
            return Mono.just(exchange)
        }

        return exchange.request.body.collectList()
            .map { buffers ->
                val totalBytes = buffers.sumOf { it.readableByteCount() }
                val bytes = ByteArray(totalBytes)
                var offset = 0
                buffers.forEach { buffer ->
                    val size = buffer.readableByteCount()
                    buffer.read(bytes, offset, size)
                    offset += size
                    DataBufferUtils.release(buffer)
                }
                context.requestBody = bytes.toString(StandardCharsets.UTF_8)
                ApiLogContext.attach(
                    exchange.mutate().request(cachedRequest(exchange, bytes)).build(),
                    context
                )
            }
            .defaultIfEmpty(exchange)
    }

    private fun shouldCaptureRequestBody(exchange: ServerWebExchange): Boolean {
        val method = exchange.request.method ?: return false
        if (method in setOf(HttpMethod.GET, HttpMethod.HEAD, HttpMethod.OPTIONS)) {
            return false
        }

        val contentType = exchange.request.headers.contentType ?: return true
        return !SKIPPED_REQUEST_MEDIA_TYPES.any { contentType.isCompatibleWith(it) }
    }

    private fun cachedRequest(exchange: ServerWebExchange, body: ByteArray): ServerHttpRequestDecorator {
        return object : ServerHttpRequestDecorator(exchange.request) {
            override fun getBody(): Flux<DataBuffer> {
                return Flux.defer {
                    Mono.just(exchange.response.bufferFactory().wrap(body))
                }
            }
        }
    }

    private fun resolveAuthentication(exchange: ServerWebExchange): Mono<Optional<Authentication>> {
        return exchange.getPrincipal<Principal>()
            .map { Optional.ofNullable(it as? Authentication) }
            .switchIfEmpty(Mono.just(Optional.empty()))
            .onErrorReturn(Optional.empty())
    }

    private class LoggingResponseDecorator(
        private val exchange: ServerWebExchange,
        private val context: ApiLogContext
    ) : ServerHttpResponseDecorator(exchange.response) {

        override fun writeWith(body: Publisher<out DataBuffer>): Mono<Void> {
            if (shouldSkipResponseCapture()) {
                return super.writeWith(body)
            }

            return DataBufferUtils.join(Flux.from(body))
                .flatMap { buffer ->
                    val bytes = ByteArray(buffer.readableByteCount())
                    buffer.read(bytes)
                    DataBufferUtils.release(buffer)
                    context.responseBody = bytes.toString(StandardCharsets.UTF_8)
                    context.responseTimestamp = java.time.OffsetDateTime.now(java.time.ZoneId.of("Asia/Seoul")).toString()
                    super.writeWith(Mono.just(bufferFactory().wrap(bytes)))
                }
                .switchIfEmpty(super.writeWith(Mono.empty()))
        }

        override fun writeAndFlushWith(body: Publisher<out Publisher<out DataBuffer>>): Mono<Void> {
            return writeWith(Flux.from(body).flatMapSequential { publisher -> publisher })
        }

        private fun shouldSkipResponseCapture(): Boolean {
            val contentType = headers.contentType ?: return false
            return RequestResponseLoggingFilter.SKIPPED_RESPONSE_MEDIA_TYPES.any { contentType.isCompatibleWith(it) }
        }
    }

    companion object {
        private val SKIPPED_REQUEST_MEDIA_TYPES = listOf(
            MediaType.MULTIPART_FORM_DATA,
            MediaType.APPLICATION_OCTET_STREAM,
            MediaType.IMAGE_JPEG,
            MediaType.IMAGE_PNG
        )

        private val SKIPPED_RESPONSE_MEDIA_TYPES = listOf(
            MediaType.TEXT_EVENT_STREAM,
            MediaType.APPLICATION_OCTET_STREAM,
            MediaType.IMAGE_JPEG,
            MediaType.IMAGE_PNG,
            MediaType.MULTIPART_FORM_DATA
        )
    }
}
