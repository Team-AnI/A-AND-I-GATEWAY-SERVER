package com.aandi.gateway.security

import org.springframework.http.HttpMethod

internal object ReportEndpointPolicyCatalog {
    val openApiRules = listOf(
        rule(HttpMethod.GET, "/v2/report/v3/api-docs"),
        rule(HttpMethod.GET, "/v2/report/v3/api-docs/**")
    )

    val serviceRules = listOf(
        rule(HttpMethod.GET, "/v1/report"),
        rule(HttpMethod.GET, "/v1/report/**"),
        rule(HttpMethod.POST, "/v1/report"),
        rule(HttpMethod.POST, "/v1/report/**"),
        rule(HttpMethod.PUT, "/v1/report/**"),
        rule(HttpMethod.PATCH, "/v1/report/**"),
        rule(HttpMethod.DELETE, "/v1/report/**"),
        rule(HttpMethod.GET, "/v2/report"),
        rule(HttpMethod.GET, "/v2/report/**"),
        rule(HttpMethod.POST, "/v2/report"),
        rule(HttpMethod.POST, "/v2/report/**"),
        rule(HttpMethod.PUT, "/v2/report/**"),
        rule(HttpMethod.PATCH, "/v2/report/**"),
        rule(HttpMethod.DELETE, "/v2/report/**")
    )

    val allowRules: List<EndpointPolicyContract> = openApiRules + serviceRules

    internal val openApiAllowRules = openApiRules.map(EndpointPolicyContract::toAllowRule)
    internal val serviceAllowRules = serviceRules.map(EndpointPolicyContract::toAllowRule)

    private fun rule(method: HttpMethod, path: String): EndpointPolicyContract {
        return EndpointPolicyContract(method, path)
    }
}
