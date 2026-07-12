package com.aandi.gateway.security

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponseWriter
import io.netty.util.NetUtil
import org.slf4j.LoggerFactory
import org.springframework.core.Ordered
import org.springframework.http.HttpMethod
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
    private val policy: SecurityPolicyProperties,
    private val responseWriter: GatewayResponseWriter
) : WebFilter, Ordered {

    private val log = LoggerFactory.getLogger(javaClass)

    private val parser = PathPatternParser.defaultInstance

    private val normalizedAllowedHosts = policy.allowedHosts.map { it.lowercase() }.toSet()

    private val jsonContentTypeExemptions: List<PathPattern> = listOf(
        parser.parse("/v1/me"),
        parser.parse("/v1/posts"),
        parser.parse("/v1/posts/{postId}"),
        parser.parse("/v1/posts/images"),
        parser.parse("/v2/me"),
        parser.parse("/v2/post"),
        parser.parse("/v2/post/{postId}"),
        parser.parse("/v2/post/images"),
        parser.parse("/v2/post/images/**"),
        parser.parse("/v2/posts"),
        parser.parse("/v2/posts/{postId}"),
        parser.parse("/v2/posts/images"),
        parser.parse("/v2/blogs"),
        parser.parse("/v2/blogs/{blogId}"),
        parser.parse("/v2/lectures"),
        parser.parse("/v2/lectures/{lectureId}")
    )

    private val actuatorHealthPaths: List<PathPattern> = listOf(
        parser.parse("/actuator/health"),
        parser.parse("/actuator/health/**")
    )

    private val allowRules: List<AllowRule> = AuthEndpointPolicyCatalog.legacyAllowRules + listOf(
        AllowRule(HttpMethod.GET, parser.parse("/v1/posts")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/posts/drafts")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/posts/drafts/**")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/posts")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/posts/{postId}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v1/posts/{postId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v1/posts/{postId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/admin/courses")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/admin/courses")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v1/admin/courses/{courseSlug}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v1/admin/courses/{courseSlug}")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/admin/courses/{courseSlug}/weeks")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/admin/courses/{courseSlug}/enrollments")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/admin/courses/{courseSlug}/enrollments")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v1/admin/courses/{courseSlug}/enrollments/{userId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v1/admin/courses/{courseSlug}/enrollments/{userId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/admin/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/admin/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v1/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v1/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/{courseSlug}")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/{courseSlug}/outline")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/{courseSlug}/weeks")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/{courseSlug}/weeks/{weekNo}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.POST, parser.parse("/v1/courses/{courseSlug}/assignments/{assignmentId}/submissions")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/{courseSlug}/assignments/{assignmentId}/submissions/{submissionId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/{courseSlug}/assignments/{assignmentId}/submissions/{submissionId}/stream")),
        AllowRule(HttpMethod.GET, parser.parse("/v1/courses/assignments/{assignmentId}/course"))
    ) + OnlineJudgeEndpointPolicyCatalog.legacyAllowRules + listOf(
        AllowRule(HttpMethod.POST, parser.parse("/v1/posts/images")),
        AllowRule(HttpMethod.GET, parser.parse("/api/ping/**"))
    ) + AuthEndpointPolicyCatalog.pingAllowRules + listOf(
        AllowRule(HttpMethod.GET, parser.parse("/")),
        AllowRule(HttpMethod.GET, parser.parse("/index.html")),
        AllowRule(HttpMethod.GET, parser.parse("/v3/api-docs")),
        AllowRule(HttpMethod.GET, parser.parse("/v3/api-docs/**")),
        AllowRule(HttpMethod.GET, parser.parse("/swagger-ui.html")),
        AllowRule(HttpMethod.GET, parser.parse("/swagger-ui/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/docs")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/docs/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/swagger-ui/index.html")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/swagger-ui/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/v3/api-docs")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/v3/api-docs/**"))
    ) + ReportEndpointPolicyCatalog.openApiAllowRules + AuthEndpointPolicyCatalog.openApiAllowRules +
        OnlineJudgeEndpointPolicyCatalog.openApiAllowRules + listOf(
        AllowRule(HttpMethod.GET, parser.parse("/actuator/health")),
        AllowRule(HttpMethod.GET, parser.parse("/actuator/health/**")),
        AllowRule(HttpMethod.POST, parser.parse("/internal/v1/cache/invalidation"))
    ) + AuthEndpointPolicyCatalog.v2AllowRules + listOf(
        AllowRule(HttpMethod.GET, parser.parse("/v2/post")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/drafts")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/drafts/**")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/{postId}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/post/{postId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/post/{postId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/admin/courses")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post/admin/courses")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/post/admin/courses/{courseSlug}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/post/admin/courses/{courseSlug}")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post/admin/courses/{courseSlug}/weeks")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/admin/courses/{courseSlug}/enrollments")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post/admin/courses/{courseSlug}/enrollments")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/post/admin/courses/{courseSlug}/enrollments/{userId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/post/admin/courses/{courseSlug}/enrollments/{userId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/admin/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post/admin/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/post/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/post/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/{courseSlug}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/{courseSlug}/outline")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/{courseSlug}/weeks")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/{courseSlug}/weeks/{weekNo}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post/courses/{courseSlug}/assignments/{assignmentId}/submissions")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/{courseSlug}/assignments/{assignmentId}/submissions/{submissionId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/{courseSlug}/assignments/{assignmentId}/submissions/{submissionId}/stream")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/courses/assignments/{assignmentId}/course")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/admin/courses")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/admin/courses")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/admin/courses/{courseSlug}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/admin/courses/{courseSlug}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/admin/courses/{courseSlug}/enrollments")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/admin/courses/{courseSlug}/enrollments")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/admin/courses/{courseSlug}/enrollments/{userId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/admin/courses/{courseSlug}/enrollments/{userId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/admin/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/admin/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/admin/courses/{courseSlug}/assignments/copy")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/admin/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/admin/courses/{courseSlug}/assignments/{assignmentId}/submission-statuses")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/courses")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/courses/{courseSlug}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/courses/{courseSlug}/outline")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/courses/{courseSlug}/weeks")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/courses/{courseSlug}/weeks/{weekNo}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/courses/{courseSlug}/assignments")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/courses/{courseSlug}/assignments/{assignmentId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/assignments/{assignmentId}/course"))
    ) + ReportEndpointPolicyCatalog.serviceAllowRules + listOf(
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/images")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/post/images/**")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post/images")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/post/images/**")),
        AllowRule(HttpMethod.PUT, parser.parse("/v2/post/images")),
        AllowRule(HttpMethod.PUT, parser.parse("/v2/post/images/**")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/post/images")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/post/images/**")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/post/images")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/post/images/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/posts")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/posts")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/posts/me")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/posts/scheduled/me")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/posts/drafts")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/posts/drafts/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/posts/{postId}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/posts/{postId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/posts/{postId}")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/posts/{postId}/collaborators")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/posts/images")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/blogs")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/blogs")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/blogs/me")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/blogs/scheduled/me")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/blogs/drafts")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/blogs/drafts/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/blogs/{blogId}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/blogs/{blogId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/blogs/{blogId}")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/lectures")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/lectures")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/lectures/me")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/lectures/scheduled/me")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/lectures/drafts")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/lectures/drafts/**")),
        AllowRule(HttpMethod.GET, parser.parse("/v2/lectures/{lectureId}")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/lectures/{lectureId}")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/lectures/{lectureId}"))
    ) + OnlineJudgeEndpointPolicyCatalog.v2AllowRules + listOf(
        AllowRule(HttpMethod.GET, parser.parse("/v2/admin/service-availability")),
        AllowRule(HttpMethod.PUT, parser.parse("/v2/admin/service-availability/{service}"))
    )

    private val denyRules: List<AllowRule> = listOf(
        AllowRule(HttpMethod.GET, parser.parse("/v2/admin/courses/{courseSlug}/assignments/copy")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/admin/courses/{courseSlug}/assignments/copy")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/admin/courses/{courseSlug}/assignments/copy")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/post/courses")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/post/courses")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/report/v3/api-docs")),
        AllowRule(HttpMethod.POST, parser.parse("/v2/report/v3/api-docs/**")),
        AllowRule(HttpMethod.PUT, parser.parse("/v2/report/v3/api-docs")),
        AllowRule(HttpMethod.PUT, parser.parse("/v2/report/v3/api-docs/**")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/report/v3/api-docs")),
        AllowRule(HttpMethod.PATCH, parser.parse("/v2/report/v3/api-docs/**")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/report/v3/api-docs")),
        AllowRule(HttpMethod.DELETE, parser.parse("/v2/report/v3/api-docs/**"))
    )

    internal val methodPathPolicy = MethodPathPolicyEvaluator(allowRules, denyRules)

    override fun getOrder(): Int = Ordered.HIGHEST_PRECEDENCE + 20

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        val request = exchange.request
        val path = request.path.pathWithinApplication()

        if (request.method == HttpMethod.OPTIONS) {
            return chain.filter(exchange)
        }

        if (isActuatorHealth(request.method, path)) {
            return chain.filter(exchange)
        }

        if (policy.enforceHttps && !isHttps(exchange)) {
            log.warn(
                "Rejecting request due to HTTPS policy: method={}, path={}, host={}, forwardedProto={}, remoteAddress={}",
                request.method,
                path.value(),
                request.headers.host?.hostString,
                request.headers.getFirst("X-Forwarded-Proto"),
                request.remoteAddress?.address?.hostAddress
            )
            return reject(exchange, GatewayErrorCode.HTTPS_REQUIRED)
        }

        if (normalizedAllowedHosts.isNotEmpty()) {
            val host = request.headers.host?.hostString?.lowercase().orEmpty()
            val hostAllowed = host in normalizedAllowedHosts ||
                (policy.allowPrivateIpHost && isLoopbackOrSiteLocalIpLiteral(host))
            if (host.isBlank() || !hostAllowed) {
                log.warn(
                    "Rejecting request due to host policy: method={}, path={}, host={}, allowedHosts={}, allowPrivateIpHost={}, remoteAddress={}",
                    request.method,
                    path.value(),
                    request.headers.host?.hostString,
                    normalizedAllowedHosts,
                    policy.allowPrivateIpHost,
                    request.remoteAddress?.address?.hostAddress
                )
                return reject(exchange, GatewayErrorCode.HOST_NOT_ALLOWED)
            }
        }

        if (policy.enforceMethodPathAllowlist) {
            when (methodPathPolicy.evaluate(request.method, path)) {
                MethodPathDecision.ALLOW -> Unit
                MethodPathDecision.EXPLICIT_DENY -> {
                    log.warn(
                        "Rejecting request due to explicit deny policy: method={}, path={}, host={}, remoteAddress={}",
                        request.method,
                        path.value(),
                        request.headers.host?.hostString,
                        request.remoteAddress?.address?.hostAddress
                    )
                    return reject(exchange, GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED)
                }

                MethodPathDecision.NO_MATCH -> {
                    log.warn(
                        "Rejecting request due to allowlist policy: method={}, path={}, host={}, remoteAddress={}",
                        request.method,
                        path.value(),
                        request.headers.host?.hostString,
                        request.remoteAddress?.address?.hostAddress
                    )
                    return reject(exchange, GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED)
                }
            }
        }

        if (policy.enforceJsonContentType && requiresJsonContentType(request.method) && !isJsonRequest(request, path)) {
            return reject(exchange, GatewayErrorCode.JSON_CONTENT_TYPE_REQUIRED)
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

    private fun isActuatorHealth(method: HttpMethod?, path: org.springframework.http.server.PathContainer): Boolean {
        return method == HttpMethod.GET && actuatorHealthPaths.any { it.matches(path) }
    }

    private fun reject(exchange: ServerWebExchange, errorCode: GatewayErrorCode): Mono<Void> {
        return responseWriter.writeError(exchange, errorCode)
    }

}

internal fun isLoopbackOrSiteLocalIpLiteral(host: String): Boolean {
    if ('%' in host) return false

    val addressBytes = NetUtil.createByteArrayFromIpAddressString(host) ?: return false
    val address = InetAddress.getByAddress(addressBytes)
    return address.isLoopbackAddress || address.isSiteLocalAddress
}
