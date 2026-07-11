package com.aandi.gateway.security

import org.springframework.http.HttpMethod
import org.springframework.http.server.PathContainer
import org.springframework.web.util.pattern.PathPattern
import org.springframework.web.util.pattern.PathPatternParser

internal data class AccessContract(
    val method: HttpMethod?,
    val paths: List<String>,
    val requirement: AccessRequirement
) {
    private val pathPatterns: List<PathPattern> = paths.map(PathPatternParser.defaultInstance::parse)

    init {
        require(paths.isNotEmpty()) { "Access paths must not be empty" }
    }

    fun matches(requestMethod: HttpMethod?, path: PathContainer): Boolean {
        return (method == null || method == requestMethod) && pathPatterns.any { it.matches(path) }
    }
}

/**
 * Auth endpoint projection of the live ordered access rules. Shared global matchers must be
 * reviewed before this test-only catalog can become a runtime source.
 */
internal object AuthAccessPolicyCatalog {
    private val userOrganizerAdmin = AccessRequirement.AnyRole(UserRole.entries.toSet())
    private val organizerAdmin = AccessRequirement.AnyRole(setOf(UserRole.ORGANIZER, UserRole.ADMIN))
    private val admin = AccessRequirement.AnyRole(setOf(UserRole.ADMIN))

    val contracts: List<AccessContract> = listOf(
        access(HttpMethod.OPTIONS, AccessRequirement.PermitAll, "/**"),
        access(HttpMethod.POST, AccessRequirement.PermitAll, "/v1/auth/**"),
        access(
            HttpMethod.POST,
            AccessRequirement.PermitAll,
            "/v2/auth/login",
            "/v2/auth/refresh",
            "/v2/auth/logout",
            "/activate",
            "/v2/activate"
        ),
        access(HttpMethod.GET, AccessRequirement.PermitAll, "/v2/ping/**"),
        access(
            HttpMethod.GET,
            AccessRequirement.PermitAll,
            "/v2/auth/v3/api-docs",
            "/v2/auth/v3/api-docs/**"
        ),
        access(
            HttpMethod.GET,
            userOrganizerAdmin,
            "/v1/me",
            "/v2/auth/me",
            "/v2/me"
        ),
        access(HttpMethod.POST, userOrganizerAdmin, "/v1/me", "/v2/auth/me"),
        access(
            HttpMethod.PATCH,
            userOrganizerAdmin,
            "/v1/me",
            "/v2/auth/me",
            "/v2/me"
        ),
        access(HttpMethod.POST, userOrganizerAdmin, "/v2/me/profile-image/upload-url"),
        access(HttpMethod.POST, userOrganizerAdmin, "/v1/me/password"),
        access(HttpMethod.PATCH, userOrganizerAdmin, "/v2/me/password"),
        access(HttpMethod.GET, organizerAdmin, "/v2/users/lookup"),
        access(
            null,
            admin,
            "/v1/admin",
            "/v1/admin/**",
            "/v2/auth/admin/**",
            "/v2/admin/**"
        )
    )

    val fallback: AccessRequirement = AccessRequirement.Authenticated

    fun evaluate(method: HttpMethod?, path: String): AccessRequirement {
        val pathContainer = PathContainer.parsePath(path)
        return contracts.firstOrNull { it.matches(method, pathContainer) }?.requirement ?: fallback
    }

    private fun access(
        method: HttpMethod?,
        requirement: AccessRequirement,
        vararg paths: String
    ): AccessContract {
        return AccessContract(method, paths.toList(), requirement)
    }
}

internal fun EndpointPolicyContract.witnessPath(): String {
    return path
        .replace(pathVariable, "value")
        .replace("**", "value")
}

private val pathVariable = Regex("""\{[^/{}]+}""")
