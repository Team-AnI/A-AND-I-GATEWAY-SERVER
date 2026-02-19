package com.aandi.gateway.security

import org.springframework.core.Ordered
import org.springframework.http.HttpMethod
import org.springframework.http.HttpStatus
import org.springframework.http.MediaType
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import org.springframework.web.util.pattern.PathPattern
import org.springframework.web.util.pattern.PathPatternParser
import reactor.core.publisher.Mono
import java.net.InetAddress

@Component
class GatewayRequestPolicyFilter(
    private val policy: SecurityPolicyProperties
) : WebFilter, Ordered {

    private val parser = PathPatternParser.defaultInstance

    private val jsonContentTypeExemptions: List<PathPattern> = listOf(
        parser.parse("/v1/images"),
        parser.parse("/v2/post/images"),
        parser.parse("/v2/post/images/**")
    )

    private val allowRules: List<AllowRule> = listOf(
        AllowRule(HttpMethod.POST, parser.parse("/v1/auth/login")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/auth/refresh")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/auth/logout")),
        AllowRule(HttpMethod.POST, parser.parse("/activate")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/me/password")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/me")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v1/me")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/admin/ping")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/admin/users")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/admin/users")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/admin/users/{id}/reset-password")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v1/admin/users/{id}")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/posts")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/posts/drafts")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/posts")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/posts/{postId}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v1/posts/{postId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v1/posts/{postId}")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/images")),
        AllowRule(HttpMethod.GET, parser.parse("/api/ping/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v3/api-docs/**")),
        AllowRule(HttpMethod.GET, parser.parse("/swagger-ui.html")),
        AllowRule(HttpMethod.GET, parser.parse("/swagger-ui/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/docs")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/docs/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/swagger-ui/index.html")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/swagger-ui/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/v3/api-docs")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/v3/api-docs/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/auth/v3/api-docs")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/auth/v3/api-docs/**")),
        AllowRule(HttpMethod.GET, parser.parse("/actuator/health")),
        AllowRule(HttpMethod.GET, parser.parse("/actuator/health/**")),
        AllowRule(HttpMethod.GET, parser.parse("/actuator/info")),
        AllowRule(HttpMethod.POST, parser.parse("/internal/v1/cache/invalidation")),
        // Legacy v2 routing
        AllowRule(HttpMethod.POST, parser.parse("/v2/auth/login")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/auth/refresh")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/auth/logout")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/auth/me")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/auth/admin/ping")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/auth/admin/users")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/auth/admin/users")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/auth/admin/users/{id}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/drafts")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/{postId}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/post/{postId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/post/{postId}")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post/images"))
    )

    override fun getOrder(): Int = Ordered.HIGHEST_PRECEDENCE + 20

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        val request = exchange.request
        val path = request.path.pathWithinApplication()

        if (request.method == HttpMethod.OPTIONS) {
            return chain.filter(exchange)
        }

        if (policy.enforceHttps && !isHttps(exchange)) {
            return reject(exchange, HttpStatus.FORBIDDEN)
        }

        if (policy.allowedHosts.isNotEmpty()) {
            val host = request.headers.host?.hostString?.lowercase().orEmpty()
            val allowedHosts = policy.allowedHosts.map { it.lowercase() }.toSet()
            val hostAllowed = host in allowedHosts || (policy.allowPrivateIpHost && isPrivateIpHost(host))
            if (host.isBlank() || !hostAllowed) {
                return reject(exchange, HttpStatus.FORBIDDEN)
            }
        }

        if (policy.enforceMethodPathAllowlist && allowRules.none { it.matches(request.method, path) }) {
            return reject(exchange, HttpStatus.NOT_FOUND)
        }

        if (policy.enforceJsonContentType && requiresJsonContentType(request.method) && !isJsonRequest(request, path)) {
            return reject(exchange, HttpStatus.UNSUPPORTED_MEDIA_TYPE)
        }

        return chain.filter(exchange)
    }

    private fun isHttps(exchange: ServerWebExchange): Boolean {
        val forwardedProto = exchange.request.headers.getFirst("X-Forwarded-Proto")
        return exchange.request.sslInfo != null || forwardedProto.equals("https", ignoreCase = true)
    }

    private fun requiresJsonContentType(method: HttpMethod?): Boolean {
        return method == HttpMethod.POST || method == HttpMethod.PUT || method == HttpMethod.PATCH
    }

    private fun isJsonRequest(request: org.springframework.http.server.reactive.ServerHttpRequest, path: org.springframework.http.server.PathContainer): Boolean {
        if (jsonContentTypeExemptions.any { it.matches(path) }) {
            return true
        }
        val contentType = request.headers.contentType
        return contentType != null && (contentType.isCompatibleWith(MediaType.APPLICATION_JSON) || contentType.subtype.endsWith("+json"))
    }

    private fun reject(exchange: ServerWebExchange, status: HttpStatus): Mono<Void> {
        val response = exchange.response
        response.statusCode = status
        return response.setComplete()
    }

    private fun isPrivateIpHost(host: String): Boolean {
        return runCatching {
            val address = InetAddress.getByName(host)
            address.isSiteLocalAddress || address.isLoopbackAddress
        }.getOrDefault(false)
    }

    private data class AllowRule(
        val method: HttpMethod,
        val pathPattern: PathPattern
    ) {
        fun matches(requestMethod: HttpMethod?, requestPath: org.springframework.http.server.PathContainer): Boolean {
            return requestMethod == method && pathPattern.matches(requestPath)
        }
    }
}
