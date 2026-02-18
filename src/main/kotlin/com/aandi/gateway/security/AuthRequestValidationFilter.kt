package com.aandi.gateway.security

import com.nimbusds.jose.crypto.MACVerifier
import com.nimbusds.jwt.SignedJWT
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

@Component
class AuthRequestValidationFilter(
    private val jwtPolicy: JwtPolicyProperties,
    private val policy: SecurityPolicyProperties
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

    override fun getOrder(): Int = Ordered.HIGHEST_PRECEDENCE + 31

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        if (exchange.request.method != HttpMethod.POST) {
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
                val validation = when (pathType) {
                    PathType.LOGIN -> validateLoginBody(body)
                    PathType.REFRESH, PathType.LOGOUT -> validateRefreshBody(body)
                }

                if (validation != null) {
                    exchange.response.statusCode = validation
                    return@flatMap exchange.response.setComplete()
                }

                val decorated = exchange.mutate().request(cachedRequest(exchange, bytes)).build()
                chain.filter(decorated)
            }
    }

    private fun validateLoginBody(body: String): HttpStatus? {
        val username = extractJsonField(body, "username")
        val password = extractJsonField(body, "password")
        if (username.isBlank() || password.isBlank()) {
            return HttpStatus.BAD_REQUEST
        }
        return null
    }

    private fun validateRefreshBody(body: String): HttpStatus? {
        val refreshToken = extractJsonField(body, "refreshToken")
        if (refreshToken.isBlank()) {
            return HttpStatus.BAD_REQUEST
        }

        if (policy.prevalidateRefreshTokenType && !isRefreshToken(refreshToken)) {
            return HttpStatus.UNAUTHORIZED
        }
        return null
    }

    private fun isRefreshToken(token: String): Boolean {
        return runCatching {
            val signed = SignedJWT.parse(token)
            val verified = signed.verify(MACVerifier(jwtPolicy.secret.toByteArray(StandardCharsets.UTF_8)))
            verified && signed.jwtClaimsSet.getStringClaim("token_type") == "REFRESH"
        }.getOrDefault(false)
    }

    private fun extractJsonField(body: String, field: String): String {
        val regex = Regex("\"$field\"\\s*:\\s*\"([^\"]*)\"")
        return regex.find(body)?.groupValues?.getOrNull(1)?.trim().orEmpty()
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

    private enum class PathType {
        LOGIN,
        REFRESH,
        LOGOUT
    }
}
