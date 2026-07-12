package com.aandi.gateway.security

import org.springframework.http.HttpMethod

internal object OnlineJudgeEndpointPolicyCatalog {
    val legacyRules = listOf(
        rule(HttpMethod.GET, "/v1/problems/{problemId}/submissions/me"),
        rule(HttpMethod.GET, "/v1/admin/submissions"),
        rule(HttpMethod.GET, "/v1/admin/testcases"),
        rule(HttpMethod.POST, "/v1/submissions"),
        rule(HttpMethod.GET, "/v1/submissions/{submissionId}"),
        rule(HttpMethod.GET, "/v1/submissions/{submissionId}/stream")
    )

    val openApiRules = listOf(
        rule(HttpMethod.GET, "/v2/online-judge/v3/api-docs"),
        rule(HttpMethod.GET, "/v2/online-judge/v3/api-docs/**")
    )

    val v2Rules = listOf(
        rule(HttpMethod.POST, "/v2/online-judge/submissions"),
        rule(HttpMethod.GET, "/v2/online-judge/problems/{problemId}/submissions/me"),
        rule(HttpMethod.GET, "/v2/online-judge/admin/submissions"),
        rule(HttpMethod.GET, "/v2/online-judge/admin/testcases"),
        rule(HttpMethod.GET, "/v2/online-judge/submissions/{submissionId}"),
        rule(HttpMethod.GET, "/v2/online-judge/submissions/{submissionId}/stream"),
        rule(HttpMethod.POST, "/v2/submissions"),
        rule(HttpMethod.GET, "/v2/problems/{problemId}/submissions/me"),
        rule(HttpMethod.GET, "/v2/admin/submissions"),
        rule(HttpMethod.GET, "/v2/admin/testcases"),
        rule(HttpMethod.GET, "/v2/submissions/{submissionId}"),
        rule(HttpMethod.GET, "/v2/submissions/{submissionId}/stream")
    )

    val allowRules: List<EndpointPolicyContract> = legacyRules + openApiRules + v2Rules

    internal val legacyAllowRules = legacyRules.map(EndpointPolicyContract::toAllowRule)
    internal val openApiAllowRules = openApiRules.map(EndpointPolicyContract::toAllowRule)
    internal val v2AllowRules = v2Rules.map(EndpointPolicyContract::toAllowRule)

    private fun rule(method: HttpMethod, path: String): EndpointPolicyContract {
        return EndpointPolicyContract(method, path)
    }
}
