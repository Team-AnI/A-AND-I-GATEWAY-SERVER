package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.core.io.buffer.DataBufferUtils
import org.springframework.core.io.buffer.DefaultDataBufferFactory
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.util.unit.DataSize
import reactor.core.publisher.Flux
import java.nio.charset.StandardCharsets
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.assertEquals
import kotlin.test.assertIs

class AuthRequestBodyCacheTests {

    @Test
    fun `chunked auth body is read once and replayed from the shared cache`() {
        val subscriptions = AtomicInteger()
        val bufferFactory = DefaultDataBufferFactory.sharedInstance
        val body = Flux.defer {
            subscriptions.incrementAndGet()
            Flux.just(
                bufferFactory.wrap("hello".toByteArray()),
                bufferFactory.wrap("-world".toByteArray())
            )
        }
        val exchange = MockServerWebExchange.from(
            MockServerHttpRequest.post("/v1/auth/login").body(body)
        )
        val cache = AuthRequestBodyCache(
            SecurityPolicyProperties(maxRequestBodySize = DataSize.ofKilobytes(1))
        )

        val first = assertIs<AuthRequestBodyReadResult.Available>(cache.read(exchange).block())
        val second = assertIs<AuthRequestBodyReadResult.Available>(cache.read(exchange).block())
        val replayed = DataBufferUtils.join(cache.decorate(exchange, first.bytes).request.body)
            .map { buffer ->
                try {
                    val bytes = ByteArray(buffer.readableByteCount())
                    buffer.read(bytes)
                    bytes.toString(StandardCharsets.UTF_8)
                } finally {
                    DataBufferUtils.release(buffer)
                }
            }
            .block()

        assertEquals("hello-world", first.bytes.toString(StandardCharsets.UTF_8))
        assertEquals("hello-world", second.bytes.toString(StandardCharsets.UTF_8))
        assertEquals("hello-world", replayed)
        assertEquals(1, subscriptions.get())
    }
}
