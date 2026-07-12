package com.aandi.gateway.security

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponseWriter
import com.nimbusds.jose.crypto.MACVerifier
import com.nimbusds.jwt.SignedJWT
import org.springframework.core.Ordered
import org.springframework.http.HttpMethod
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import org.springframework.web.util.pattern.PathPatternParser
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets

@Component
class AuthRequestValidationFilter(
    private val jwtPolicy: JwtPolicyProperties,
    private val policy: SecurityPolicyProperties,
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

        return requestBodyCache.read(exchange)
            .flatMap { result ->
                val bytes = when (result) {
                    is AuthRequestBodyReadResult.Available -> result.bytes
                    AuthRequestBodyReadResult.TooLarge -> {
                        return@flatMap responseWriter.writeError(exchange, GatewayErrorCode.REQUEST_BODY_TOO_LARGE)
                    }
                }
                val body = bytes.toString(StandardCharsets.UTF_8)
                val validation = when (pathType) {
                    PathType.LOGIN -> validateLoginBody(body)
                    PathType.REFRESH, PathType.LOGOUT -> validateRefreshBody(exchange, body)
                }

                if (validation != null) {
                    return@flatMap responseWriter.writeError(exchange, validation)
                }

                val decorated = requestBodyCache.decorate(exchange, bytes)
                chain.filter(decorated)
            }
    }

    private fun validateLoginBody(body: String): GatewayErrorCode? {
        val username = extractJsonField(body, "username")
        val password = extractJsonField(body, "password")
        if (username.isBlank() || password.isBlank()) {
            return GatewayErrorCode.LOGIN_REQUEST_BODY_INVALID
        }
        return null
    }

    private fun validateRefreshBody(exchange: ServerWebExchange, body: String): GatewayErrorCode? {
        val refreshToken = extractJsonField(body, "refreshToken")
            .ifBlank { exchange.request.cookies.getFirst("refresh_token")?.value }

        if (refreshToken.isNullOrBlank()) {
            return GatewayErrorCode.REFRESH_TOKEN_REQUIRED
        }

        if (policy.prevalidateRefreshTokenType && !isRefreshToken(refreshToken)) {
            return GatewayErrorCode.REFRESH_TOKEN_INVALID
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

    private enum class PathType {
        LOGIN,
        REFRESH,
        LOGOUT
    }
}
