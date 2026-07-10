package com.aandi.gateway.logging

import org.reactivestreams.Publisher
import org.springframework.core.Ordered
import org.springframework.core.io.buffer.DataBuffer
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
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.nio.charset.StandardCharsets
import java.security.Principal
import java.util.Optional

@Component
class RequestResponseLoggingFilter(
    private val apiLogFactory: ApiLogFactory,
    private val apiStructuredLogger: ApiLogSink
) : WebFilter, Ordered {

    override fun getOrder(): Int = Ordered.HIGHEST_PRECEDENCE + 5

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        val context = ApiLogContext.initialize(exchange)
        val requestExchange = withRequestBodyCapture(exchange, context)
        val tracedExchange = withTraceHeaders(requestExchange, context)
        val decoratedResponse = LoggingResponseDecorator(tracedExchange, context)
        val decoratedExchange = ApiLogContext.attach(
            tracedExchange.mutate().response(decoratedResponse).build(),
            context
        )

        return chain.filter(decoratedExchange)
            .then(resolveAuthentication(decoratedExchange))
            .doOnNext { authentication ->
                apiStructuredLogger.log(apiLogFactory.create(decoratedExchange, context, authentication.orElse(null)))
            }
            .then()
    }

    private fun withTraceHeaders(exchange: ServerWebExchange, context: ApiLogContext): ServerWebExchange {
        val request = exchange.request.mutate().headers { headers ->
            headers.set(ApiLogContext.TRACE_ID_HEADER, context.traceId)
            headers.set(ApiLogContext.REQUEST_ID_HEADER, context.requestId)
        }.build()
        return ApiLogContext.attach(exchange.mutate().request(request).build(), context)
    }

    private fun withRequestBodyCapture(exchange: ServerWebExchange, context: ApiLogContext): ServerWebExchange {
        if (!shouldCaptureRequestBody(exchange)) {
            return exchange
        }

        val capture = LimitedBodyCapture(MAX_CAPTURED_BODY_BYTES)
        val request = object : ServerHttpRequestDecorator(exchange.request) {
            override fun getBody(): Flux<DataBuffer> {
                val finishCapture = { context.requestBody = capture.text() }
                return super.getBody()
                    .doOnNext(capture::append)
                    .doOnComplete(finishCapture)
                    .doOnError { finishCapture() }
                    .doOnCancel(finishCapture)
            }
        }
        return ApiLogContext.attach(exchange.mutate().request(request).build(), context)
    }

    private fun shouldCaptureRequestBody(exchange: ServerWebExchange): Boolean {
        val method = exchange.request.method ?: return false
        if (method in setOf(HttpMethod.GET, HttpMethod.HEAD, HttpMethod.OPTIONS)) {
            return false
        }

        val contentType = exchange.request.headers.contentType ?: return true
        return !SKIPPED_REQUEST_MEDIA_TYPES.any { contentType.isCompatibleWith(it) }
    }

    private fun resolveAuthentication(exchange: ServerWebExchange): Mono<Optional<Authentication>> {
        return exchange.getPrincipal<Principal>()
            .map { Optional.ofNullable(it as? Authentication) }
            .switchIfEmpty(Mono.just(Optional.empty()))
            .onErrorReturn(Optional.empty())
    }

    private class LoggingResponseDecorator(
        exchange: ServerWebExchange,
        private val context: ApiLogContext
    ) : ServerHttpResponseDecorator(exchange.response) {
        private val capture = LimitedBodyCapture(MAX_CAPTURED_BODY_BYTES)

        override fun writeWith(body: Publisher<out DataBuffer>): Mono<Void> {
            if (shouldSkipResponseCapture()) {
                return super.writeWith(body)
            }

            return super.writeWith(capture(body))
                .doOnTerminate(this::finishCapture)
                .doOnCancel(this::finishCapture)
        }

        override fun writeAndFlushWith(body: Publisher<out Publisher<out DataBuffer>>): Mono<Void> {
            if (shouldSkipResponseCapture()) {
                return super.writeAndFlushWith(body)
            }

            val capturedBody = Flux.from(body).map { publisher -> capture(publisher) }
            return super.writeAndFlushWith(capturedBody)
                .doOnTerminate(this::finishCapture)
                .doOnCancel(this::finishCapture)
        }

        private fun capture(body: Publisher<out DataBuffer>): Flux<out DataBuffer> {
            return Flux.from(body).doOnNext(capture::append)
        }

        private fun finishCapture() {
            context.responseBody = capture.text()
            context.responseTimestamp = java.time.OffsetDateTime.now(java.time.ZoneId.of("Asia/Seoul")).toString()
        }

        private fun shouldSkipResponseCapture(): Boolean {
            val contentType = headers.contentType ?: return false
            return SKIPPED_RESPONSE_MEDIA_TYPES.any { contentType.isCompatibleWith(it) }
        }
    }

    companion object {
        private const val MAX_CAPTURED_BODY_BYTES = 64 * 1024

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

private class LimitedBodyCapture(
    private val maxBytes: Int
) {
    private val output = ByteArrayOutputStream(minOf(maxBytes, 1024))
    private var truncated = false

    fun append(buffer: DataBuffer) {
        val readableBytes = buffer.readableByteCount()
        val remainingBytes = (maxBytes - output.size()).coerceAtLeast(0)
        val copyLength = minOf(readableBytes, remainingBytes)
        if (copyLength > 0) {
            val bytes = ByteArray(copyLength)
            buffer.toByteBuffer(buffer.readPosition(), ByteBuffer.wrap(bytes), 0, copyLength)
            output.write(bytes)
        }
        if (copyLength < readableBytes) {
            truncated = true
        }
    }

    fun text(): String {
        return if (truncated) TRUNCATED_BODY_MARKER else output.toString(StandardCharsets.UTF_8)
    }
}

internal const val TRUNCATED_BODY_MARKER = "[truncated]"
