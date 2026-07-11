package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.http.HttpMethod
import org.springframework.http.HttpStatus
import org.springframework.security.web.server.WebFilterChainProxy
import kotlin.test.assertEquals

@SpringBootTest(
    properties = [
        "POST_SERVICE_URI=http://localhost:8084",
        "AUTH_SERVICE_URI=http://localhost:9000",
        "ONLINE_JUDGE_SERVICE_URI=http://localhost:8080",
        "app.security.internal-event-token=test-internal-token",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!",
        "gateway.auth.enabled=true",
        "app.security.policy.enforce-https=false"
    ]
)
class AuthAccessPolicyProjectionIntegrationTests(
    @Autowired private val springSecurity: WebFilterChainProxy
) {
    private val securityProbe = SecurityChainProbe(springSecurity)

    @Test
    fun `global access catalog projection matches the live security chain for all auth endpoint policies`() {
        val endpoints = AuthEndpointPolicyCatalog.allowRules

        assertEquals(73, endpoints.size)
        endpoints.forEach { endpoint ->
            val path = endpoint.witnessPath()
            val requirement = GlobalAccessPolicyCatalog.evaluate(endpoint.method, path).requirement

            securityProbe.assertAccess(endpoint.method, path, requirement)
        }
    }

    @Test
    fun `global options rule wins before auth role matchers`() {
        listOf(
            "/v1/users",
            "/v1/users/value",
            "/v2/auth/users",
            "/v2/auth/users/value",
            "/v2/auth/admin/users/value"
        ).forEach { path ->
            assertEquals(HttpStatus.NO_CONTENT, securityProbe.exchange(HttpMethod.OPTIONS, path), path)
        }
    }

    @Test
    fun `global access catalog matches live auth wildcard and fallback boundaries`() {
        val admin = AccessRequirement.AnyRole(setOf(UserRole.ADMIN))
        val cases = listOf(
            BoundaryCase(HttpMethod.POST, "/v1/auth/future/deep", AccessRequirement.PermitAll),
            BoundaryCase(HttpMethod.GET, "/v1/auth/login", AccessRequirement.Authenticated),
            BoundaryCase(HttpMethod.GET, "/v2/ping/future/deep", AccessRequirement.PermitAll),
            BoundaryCase(
                HttpMethod.GET,
                "/v2/auth/v3/api-docs/v1/users",
                AccessRequirement.PermitAll
            ),
            BoundaryCase(
                HttpMethod.POST,
                "/v2/auth/me/password",
                AccessRequirement.Authenticated
            ),
            BoundaryCase(HttpMethod.GET, "/v1/admin", admin),
            BoundaryCase(HttpMethod.PATCH, "/v1/admin/future/deep", admin),
            BoundaryCase(HttpMethod.DELETE, "/v2/auth/admin/future/deep", admin),
            BoundaryCase(HttpMethod.PUT, "/v2/admin/future/deep", admin),
            BoundaryCase(HttpMethod.GET, "/v2/auth/future", AccessRequirement.Authenticated)
        )

        cases.forEach { case ->
            assertEquals(
                case.requirement,
                GlobalAccessPolicyCatalog.evaluate(case.method, case.path).requirement,
                "${case.method} ${case.path} catalog"
            )
            securityProbe.assertAccess(case.method, case.path, case.requirement)
        }
    }

    private data class BoundaryCase(
        val method: HttpMethod,
        val path: String,
        val requirement: AccessRequirement
    )
}
