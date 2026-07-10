package com.aandi.gateway.security

import org.springframework.http.HttpMethod
import org.springframework.http.server.PathContainer
import org.springframework.web.util.pattern.PathPattern
import org.springframework.web.util.pattern.PathPatternParser

internal class AllowRule(
    val method: HttpMethod,
    private val pathPattern: PathPattern
) {
    constructor(method: HttpMethod, path: String) : this(
        method,
        PathPatternParser.defaultInstance.parse(path)
    )

    val path: String = pathPattern.patternString

    fun matches(requestMethod: HttpMethod?, requestPath: PathContainer): Boolean {
        return requestMethod == method && pathPattern.matches(requestPath)
    }
}

internal enum class MethodPathDecision {
    ALLOW,
    EXPLICIT_DENY,
    NO_MATCH
}

internal class MethodPathPolicyEvaluator(
    internal val allowRules: List<AllowRule>,
    internal val denyRules: List<AllowRule>
) {
    fun evaluate(method: HttpMethod?, path: PathContainer): MethodPathDecision {
        return when {
            denyRules.any { it.matches(method, path) } -> MethodPathDecision.EXPLICIT_DENY
            allowRules.any { it.matches(method, path) } -> MethodPathDecision.ALLOW
            else -> MethodPathDecision.NO_MATCH
        }
    }
}
