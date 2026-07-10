package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.http.HttpMethod
import org.springframework.http.HttpStatus
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken
import org.springframework.security.core.GrantedAuthority
import org.springframework.security.core.authority.SimpleGrantedAuthority
import org.springframework.security.core.context.ReactiveSecurityContextHolder
import org.springframework.security.web.server.WebFilterChainProxy
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilterChain
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import reactor.core.publisher.Mono
import java.net.URI
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
class AuthAccessPolicyCatalogIntegrationTests(
    @Autowired private val springSecurity: WebFilterChainProxy
) {
    private val authenticatedPrincipals = listOf(
        PrincipalCase("scope-only", listOf(SimpleGrantedAuthority("SCOPE_read")), role = null)
    ) + UserRole.entries.map { role ->
        PrincipalCase(
            role.name,
            listOf(SimpleGrantedAuthority("ROLE_${role.name}")),
            role
        )
    }

    @Test
    fun `auth access catalog matches the live security chain for all endpoint policies`() {
        val endpoints = AuthEndpointPolicyCatalog.allowRules

        assertEquals(73, endpoints.size)
        endpoints.forEach { endpoint ->
            val path = endpoint.witnessPath()
            val requirement = AuthAccessPolicyCatalog.evaluate(endpoint.method, path)

            assertLiveAccess(endpoint.method, path, requirement)
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
            assertEquals(HttpStatus.NO_CONTENT, exchange(HttpMethod.OPTIONS, path), path)
        }
    }

    @Test
    fun `auth access catalog matches live wildcard and fallback boundaries`() {
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
                AuthAccessPolicyCatalog.evaluate(case.method, case.path),
                "${case.method} ${case.path} catalog"
            )
            assertLiveAccess(case.method, case.path, case.requirement)
        }
    }

    private fun assertLiveAccess(
        method: HttpMethod,
        path: String,
        requirement: AccessRequirement
    ) {
        when (requirement) {
            AccessRequirement.PermitAll -> {
                assertEquals(HttpStatus.NO_CONTENT, exchange(method, path), "$method $path anonymous")
                authenticatedPrincipals.forEach { principal ->
                    assertEquals(
                        HttpStatus.NO_CONTENT,
                        exchange(method, path, principal.authorities),
                        "$method $path ${principal.label}"
                    )
                }
            }

            AccessRequirement.Authenticated -> {
                assertEquals(HttpStatus.UNAUTHORIZED, exchange(method, path), "$method $path anonymous")
                authenticatedPrincipals.forEach { principal ->
                    assertEquals(
                        HttpStatus.NO_CONTENT,
                        exchange(method, path, principal.authorities),
                        "$method $path ${principal.label}"
                    )
                }
            }

            is AccessRequirement.AnyRole -> {
                assertEquals(HttpStatus.UNAUTHORIZED, exchange(method, path), "$method $path anonymous")
                authenticatedPrincipals.forEach { principal ->
                    val expected = if (principal.role != null && principal.role in requirement.roles) {
                        HttpStatus.NO_CONTENT
                    } else {
                        HttpStatus.FORBIDDEN
                    }
                    assertEquals(
                        expected,
                        exchange(method, path, principal.authorities),
                        "$method $path ${principal.label}"
                    )
                }
            }
        }
    }

    private fun exchange(
        method: HttpMethod,
        path: String,
        authorities: Collection<GrantedAuthority>? = null
    ): HttpStatus {
        val request = MockServerHttpRequest.method(method, URI.create("http://localhost$path")).build()
        val exchange = MockServerWebExchange.from(request)
        val result = springSecurity.filter(exchange, terminalChain).let { filtered ->
            if (authorities == null) {
                filtered
            } else {
                val authentication = UsernamePasswordAuthenticationToken.authenticated(
                    "principal",
                    "credentials",
                    authorities
                )
                filtered.contextWrite(ReactiveSecurityContextHolder.withAuthentication(authentication))
            }
        }

        result.block()
        val status = requireNotNull(exchange.response.statusCode) {
            "Security chain did not complete the response for $method $path"
        }
        return HttpStatus.valueOf(status.value())
    }

    private companion object {
        val terminalChain = WebFilterChain { exchange: ServerWebExchange ->
            exchange.response.statusCode = HttpStatus.NO_CONTENT
            Mono.empty()
        }
    }

    private data class PrincipalCase(
        val label: String,
        val authorities: List<GrantedAuthority>,
        val role: UserRole?
    )

    private data class BoundaryCase(
        val method: HttpMethod,
        val path: String,
        val requirement: AccessRequirement
    )
}
