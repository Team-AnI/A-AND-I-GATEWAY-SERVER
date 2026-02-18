package com.aandi.gateway.security
import org.springframework.core.Ordered
import org.springframework.core.io.buffer.DataBufferUtils
import org.springframework.http.HttpMethod
import org.springframework.http.HttpStatus
import org.springframework.http.server.reactive.ServerHttpRequestDecorator
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import org.springframework.web.util.pattern.PathPatternParser
import reactor.core.publisher.Flux
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap

@Component
class AuthRateLimitFilter(
    private val properties: RateLimitProperties
) : WebFilter, Ordered {

    private val parser = PathPatternParser.defaultInstance
    private val loginPaths = listOf(
        parser.parse("/v1/auth/login"),
        parser.parse("/v2/auth/login")
    )
    private val refreshPaths = listOf(
        parser.parse("/v1/auth/refresh"),
        parser.parse("/v2/auth/refresh")
    )
    private val logoutPaths = listOf(
        parser.parse("/v1/auth/logout"),
        parser.parse("/v2/auth/logout")
    )

    private val counters = ConcurrentHashMap<String, Counter>()

    override fun getOrder(): Int = Ordered.HIGHEST_PRECEDENCE + 30

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        if (!properties.enabled || exchange.request.method != HttpMethod.POST) {
            return chain.filter(exchange)
        }

        val requestPath = exchange.request.path.pathWithinApplication()
        val pathType = when {
            loginPaths.any { it.matches(requestPath) } -> PathType.LOGIN
            refreshPaths.any { it.matches(requestPath) } -> PathType.REFRESH
            logoutPaths.any { it.matches(requestPath) } -> PathType.LOGOUT
            else -> null
        } ?: return chain.filter(exchange)

        return exchange.request.body
            .collectList()
            .flatMap { buffers ->
                val totalBytes = buffers.sumOf { it.readableByteCount() }
                val bytes = ByteArray(totalBytes)
                var offset = 0
                buffers.forEach { buffer ->
                    val size = buffer.readableByteCount()
                    buffer.read(bytes, offset, size)
                    offset += size
                    DataBufferUtils.release(buffer)
                }

                val body = bytes.toString(StandardCharsets.UTF_8)
                val key = buildKey(exchange, pathType, body)
                val limit = when (pathType) {
                    PathType.LOGIN -> properties.loginPerMinute
                    PathType.REFRESH -> properties.refreshPerMinute
                    PathType.LOGOUT -> properties.logoutPerMinute
                }

                if (!allow(key, limit)) {
                    exchange.response.statusCode = HttpStatus.TOO_MANY_REQUESTS
                    return@flatMap exchange.response.setComplete()
                }

                val decorated = exchange.mutate().request(cachedRequest(exchange, bytes)).build()
                chain.filter(decorated)
            }
    }

    private fun buildKey(exchange: ServerWebExchange, pathType: PathType, body: String): String {
        val ip = exchange.request.remoteAddress?.address?.hostAddress ?: "unknown-ip"
        return when (pathType) {
            PathType.LOGIN -> {
                val username = extractJsonField(body, "username").ifBlank { "unknown-user" }
                "login:$ip:$username"
            }
            PathType.REFRESH, PathType.LOGOUT -> {
                val refreshToken = extractJsonField(body, "refreshToken")
                val tokenHash = sha256(refreshToken.ifBlank { "unknown-refresh" })
                "refresh:$ip:$tokenHash"
            }
        }
    }

    private fun extractJsonField(body: String, field: String): String {
        val regex = Regex("\"$field\"\\s*:\\s*\"([^\"]*)\"")
        return regex.find(body)?.groupValues?.getOrNull(1)?.trim().orEmpty()
    }

    private fun allow(key: String, limit: Int): Boolean {
        if (limit <= 0) return false
        val nowEpochMinute = Instant.now().epochSecond / 60
        val next = counters.compute(key) { _, current ->
            if (current == null || current.windowMinute != nowEpochMinute) {
                Counter(nowEpochMinute, 1)
            } else {
                current.copy(count = current.count + 1)
            }
        } ?: Counter(nowEpochMinute, 1)
        return next.count <= limit
    }

    private fun cachedRequest(exchange: ServerWebExchange, body: ByteArray): ServerHttpRequestDecorator {
        return object : ServerHttpRequestDecorator(exchange.request) {
            override fun getBody(): Flux<org.springframework.core.io.buffer.DataBuffer> {
                return Flux.defer {
                    val dataBuffer = exchange.response.bufferFactory().wrap(body)
                    Mono.just(dataBuffer)
                }
            }
        }
    }

    private fun sha256(value: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(StandardCharsets.UTF_8))
        return digest.joinToString("") { "%02x".format(it) }
    }

    private data class Counter(
        val windowMinute: Long,
        val count: Int
    )

    private enum class PathType {
        LOGIN,
        REFRESH,
        LOGOUT
    }
}
