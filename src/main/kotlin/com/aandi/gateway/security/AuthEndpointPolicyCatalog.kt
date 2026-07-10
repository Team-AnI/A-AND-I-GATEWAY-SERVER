package com.aandi.gateway.security

import org.springframework.http.HttpMethod

internal data class EndpointPolicyContract(
    val method: HttpMethod,
    val path: String
) {
    fun toAllowRule(): AllowRule = AllowRule(method, path)
}

internal object AuthEndpointPolicyCatalog {
    val legacyRules = listOf(
        rule(HttpMethod.POST, "/v1/auth/login"),
        rule(HttpMethod.POST, "/v1/auth/refresh"),
        rule(HttpMethod.POST, "/v1/auth/logout"),
        rule(HttpMethod.POST, "/activate"),
        rule(HttpMethod.POST, "/v1/me/password"),
        rule(HttpMethod.GET, "/v1/me"),
        rule(HttpMethod.POST, "/v1/me"),
        rule(HttpMethod.PATCH, "/v1/me"),
        rule(HttpMethod.GET, "/v1/admin/ping"),
        rule(HttpMethod.GET, "/v1/admin/users"),
        rule(HttpMethod.POST, "/v1/admin/users"),
        rule(HttpMethod.POST, "/v1/admin/users/sync"),
        rule(HttpMethod.POST, "/v1/admin/invite-mail"),
        rule(HttpMethod.PATCH, "/v1/admin/users/role"),
        rule(HttpMethod.PATCH, "/v1/admin/users/**"),
        rule(HttpMethod.POST, "/v1/admin/users/{id}/reset-password"),
        rule(HttpMethod.DELETE, "/v1/admin/users"),
        rule(HttpMethod.DELETE, "/v1/admin/users/{id}"),
        rule(HttpMethod.GET, "/v1/users"),
        rule(HttpMethod.GET, "/v1/users/**"),
        rule(HttpMethod.POST, "/v1/users"),
        rule(HttpMethod.POST, "/v1/users/**"),
        rule(HttpMethod.PUT, "/v1/users"),
        rule(HttpMethod.PUT, "/v1/users/**"),
        rule(HttpMethod.PATCH, "/v1/users"),
        rule(HttpMethod.PATCH, "/v1/users/**"),
        rule(HttpMethod.DELETE, "/v1/users"),
        rule(HttpMethod.DELETE, "/v1/users/**")
    )

    val pingRules = listOf(
        rule(HttpMethod.GET, "/v2/ping"),
        rule(HttpMethod.GET, "/v2/ping/**")
    )

    val openApiRules = listOf(
        rule(HttpMethod.GET, "/v2/auth/v3/api-docs"),
        rule(HttpMethod.GET, "/v2/auth/v3/api-docs/**")
    )

    val v2Rules = listOf(
        rule(HttpMethod.POST, "/v2/auth/login"),
        rule(HttpMethod.POST, "/v2/auth/refresh"),
        rule(HttpMethod.POST, "/v2/auth/logout"),
        rule(HttpMethod.POST, "/v2/activate"),
        rule(HttpMethod.GET, "/v2/auth/me"),
        rule(HttpMethod.POST, "/v2/auth/me"),
        rule(HttpMethod.PATCH, "/v2/auth/me"),
        rule(HttpMethod.POST, "/v2/auth/me/password"),
        rule(HttpMethod.GET, "/v2/auth/admin/ping"),
        rule(HttpMethod.GET, "/v2/auth/admin/users"),
        rule(HttpMethod.POST, "/v2/auth/admin/users"),
        rule(HttpMethod.POST, "/v2/auth/admin/users/sync"),
        rule(HttpMethod.POST, "/v2/auth/admin/invite-mail"),
        rule(HttpMethod.PATCH, "/v2/auth/admin/users/role"),
        rule(HttpMethod.PATCH, "/v2/auth/admin/users/**"),
        rule(HttpMethod.POST, "/v2/auth/admin/users/{id}/reset-password"),
        rule(HttpMethod.DELETE, "/v2/auth/admin/users"),
        rule(HttpMethod.DELETE, "/v2/auth/admin/users/{id}"),
        rule(HttpMethod.GET, "/v2/auth/users"),
        rule(HttpMethod.GET, "/v2/auth/users/**"),
        rule(HttpMethod.POST, "/v2/auth/users"),
        rule(HttpMethod.POST, "/v2/auth/users/**"),
        rule(HttpMethod.PUT, "/v2/auth/users"),
        rule(HttpMethod.PUT, "/v2/auth/users/**"),
        rule(HttpMethod.PATCH, "/v2/auth/users"),
        rule(HttpMethod.PATCH, "/v2/auth/users/**"),
        rule(HttpMethod.DELETE, "/v2/auth/users"),
        rule(HttpMethod.DELETE, "/v2/auth/users/**"),
        rule(HttpMethod.GET, "/v2/me"),
        rule(HttpMethod.PATCH, "/v2/me"),
        rule(HttpMethod.POST, "/v2/me/profile-image/upload-url"),
        rule(HttpMethod.PATCH, "/v2/me/password"),
        rule(HttpMethod.GET, "/v2/users/lookup"),
        rule(HttpMethod.GET, "/v2/admin/ping"),
        rule(HttpMethod.GET, "/v2/admin/users"),
        rule(HttpMethod.POST, "/v2/admin/users"),
        rule(HttpMethod.POST, "/v2/admin/invite-mail"),
        rule(HttpMethod.POST, "/v2/admin/users/{id}/password/reset"),
        rule(HttpMethod.PATCH, "/v2/admin/users/{id}/role"),
        rule(HttpMethod.PATCH, "/v2/admin/users/{id}"),
        rule(HttpMethod.DELETE, "/v2/admin/users/{id}")
    )

    val allowRules: List<EndpointPolicyContract> = legacyRules + pingRules + openApiRules + v2Rules

    internal val legacyAllowRules = legacyRules.map(EndpointPolicyContract::toAllowRule)
    internal val pingAllowRules = pingRules.map(EndpointPolicyContract::toAllowRule)
    internal val openApiAllowRules = openApiRules.map(EndpointPolicyContract::toAllowRule)
    internal val v2AllowRules = v2Rules.map(EndpointPolicyContract::toAllowRule)

    private fun rule(method: HttpMethod, path: String): EndpointPolicyContract {
        return EndpointPolicyContract(method, path)
    }
}
