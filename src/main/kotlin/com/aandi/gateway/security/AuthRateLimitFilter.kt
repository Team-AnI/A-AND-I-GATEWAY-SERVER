package com.aandi.gateway.security
import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponseWriter
import org.springframework.core.Ordered
import org.springframework.http.HttpMethod
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import org.springframework.web.util.pattern.PathPatternParser
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets
import java.security.MessageDigest

@Component
class AuthRateLimitFilter(
    private val properties: RateLimitProperties,
    private val responseWriter: GatewayResponseWriter,
    private val requestBodyCache: AuthRequestBodyCache
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

    private val rateLimiter = FixedWindowRateLimiter(properties.counterSlots)

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

        return requestBodyCache.read(exchange)
            .flatMap { result ->
                val bytes = when (result) {
                    is AuthRequestBodyReadResult.Available -> result.bytes
                    AuthRequestBodyReadResult.TooLarge -> {
                        return@flatMap responseWriter.writeError(exchange, GatewayErrorCode.REQUEST_BODY_TOO_LARGE)
                    }
                }
                val body = bytes.toString(StandardCharsets.UTF_8)
                val key = buildKey(exchange, pathType, body)
                val limit = when (pathType) {
                    PathType.LOGIN -> properties.loginPerMinute
                    PathType.REFRESH -> properties.refreshPerMinute
                    PathType.LOGOUT -> properties.logoutPerMinute
                }

                if (!rateLimiter.allow(key, limit)) {
                    val errorCode = when (pathType) {
                        PathType.LOGIN -> GatewayErrorCode.LOGIN_RATE_LIMIT_EXCEEDED
                        PathType.REFRESH -> GatewayErrorCode.REFRESH_RATE_LIMIT_EXCEEDED
                        PathType.LOGOUT -> GatewayErrorCode.LOGOUT_RATE_LIMIT_EXCEEDED
                    }
                    return@flatMap responseWriter.writeError(exchange, errorCode)
                }

                val decorated = requestBodyCache.decorate(exchange, bytes)
                chain.filter(decorated)
            }
    }

    private fun buildKey(exchange: ServerWebExchange, pathType: PathType, body: String): String {
        val ip = exchange.request.remoteAddress?.address?.hostAddress ?: "unknown-ip"
        return when (pathType) {
            PathType.LOGIN -> {
                val username = extractJsonField(body, "username").ifBlank { "unknown-user" }
                "login:$ip:${sha256(username)}"
            }
            PathType.REFRESH, PathType.LOGOUT -> {
                val refreshToken = extractJsonField(body, "refreshToken")
                    .ifBlank { exchange.request.cookies.getFirst("refresh_token")?.value ?: "" }
                val tokenHash = sha256(refreshToken.ifBlank { "unknown-refresh" })
                "refresh:$ip:$tokenHash"
            }
        }
    }

    private fun extractJsonField(body: String, field: String): String {
        val regex = Regex("\"$field\"\\s*:\\s*\"([^\"]*)\"")
        return regex.find(body)?.groupValues?.getOrNull(1)?.trim().orEmpty()
    }

    private fun sha256(value: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(StandardCharsets.UTF_8))
        return digest.joinToString("") { "%02x".format(it) }
    }

    private enum class PathType {
        LOGIN,
        REFRESH,
        LOGOUT
    }
}
