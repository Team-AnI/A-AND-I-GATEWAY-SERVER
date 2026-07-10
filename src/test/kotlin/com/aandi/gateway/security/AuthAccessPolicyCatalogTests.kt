package com.aandi.gateway.security

import com.aandi.gateway.routing.AuthGatewayRouteCatalog
import org.junit.jupiter.api.Test
import org.springframework.http.HttpMethod
import org.springframework.http.server.PathContainer
import org.springframework.web.util.pattern.PathPatternParser
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.util.HexFormat
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class AuthAccessPolicyCatalogTests {

    @Test
    fun `auth access catalog preserves ordered matcher inventory`() {
        val contracts = AuthAccessPolicyCatalog.contracts

        assertEquals(13, contracts.size)
        assertEquals(26, contracts.sumOf { it.paths.size })
        assertEquals(
            listOf(
                HttpMethod.OPTIONS,
                HttpMethod.POST,
                HttpMethod.POST,
                HttpMethod.GET,
                HttpMethod.GET,
                HttpMethod.GET,
                HttpMethod.POST,
                HttpMethod.PATCH,
                HttpMethod.POST,
                HttpMethod.POST,
                HttpMethod.PATCH,
                HttpMethod.GET,
                null
            ),
            contracts.map { it.method }
        )
        assertEquals(AccessRequirement.Authenticated, AuthAccessPolicyCatalog.fallback)

        val orderedKeys = contracts.flatMap { contract ->
            contract.paths.map { path ->
                "${contract.method?.name() ?: "*"} $path ${contract.requirement.canonicalName()}"
            }
        } + "* /** ${AuthAccessPolicyCatalog.fallback.canonicalName()}"
        assertEquals(27, orderedKeys.size)
        assertEquals(
            "93394a0ddddc719a921d4e6f298585fe5636a1582e37a83e4f4051b1ab5e698c",
            sha256(orderedKeys.joinToString("\n"))
        )
    }

    @Test
    fun `auth access projection contains only routed endpoints and keeps methodless routes explicit`() {
        val routes = AuthGatewayRouteCatalog.catalog.routes
        val parser = PathPatternParser.defaultInstance
        val routeMatchers = routes.map { route -> route to parser.parse(route.path.value) }

        AuthEndpointPolicyCatalog.allowRules.forEach { endpoint ->
            val witnessPath = PathContainer.parsePath(endpoint.witnessPath())
            assertTrue(
                routeMatchers.any { (route, pathPattern) ->
                    route.enabled &&
                        (route.method == null || route.method == endpoint.method) &&
                        pathPattern.matches(witnessPath)
                },
                "No Auth route owns ${endpoint.method} ${endpoint.path}"
            )
        }

        val methodlessRoutePaths = routes
            .filter { it.method == null }
            .map { it.path.value }

        assertEquals(
            listOf(
                "/v1/users",
                "/v1/users/**",
                "/v2/auth/users",
                "/v2/auth/users/**"
            ),
            methodlessRoutePaths
        )
    }

    @Test
    fun `auth endpoint policies keep their effective access distribution`() {
        val endpointAccess = AuthEndpointPolicyCatalog.allowRules.map { endpoint ->
            EndpointAccess(
                method = endpoint.method,
                path = endpoint.path,
                requirement = AuthAccessPolicyCatalog.evaluate(endpoint.method, endpoint.witnessPath())
            )
        }
        val requirements = endpointAccess.map { it.requirement }

        assertEquals(73, requirements.size)
        assertEquals(
            mapOf(
                AccessRequirement.PermitAll to 12,
                AccessRequirement.Authenticated to 21,
                roles(UserRole.USER, UserRole.ORGANIZER, UserRole.ADMIN) to 11,
                roles(UserRole.ORGANIZER, UserRole.ADMIN) to 1,
                roles(UserRole.ADMIN) to 28
            ),
            requirements.groupingBy { it }.eachCount()
        )
        assertEquals(
            "039d9c0904087244a8d3c84bcf7a0166c3bf8ceac7d886e98383db2a56d666e0",
            fingerprint(endpointAccess)
        )
    }

    @Test
    fun `first match access decisions preserve global and fallback exceptions`() {
        val cases = listOf(
            AccessCase(HttpMethod.OPTIONS, "/v2/auth/admin/users/value", AccessRequirement.PermitAll),
            AccessCase(HttpMethod.POST, "/v1/auth/future", AccessRequirement.PermitAll),
            AccessCase(HttpMethod.GET, "/v2/ping", AccessRequirement.PermitAll),
            AccessCase(HttpMethod.GET, "/v2/ping/deep", AccessRequirement.PermitAll),
            AccessCase(HttpMethod.GET, "/v2/auth/v3/api-docs/v1", AccessRequirement.PermitAll),
            AccessCase(
                HttpMethod.POST,
                "/v2/auth/me/password",
                AccessRequirement.Authenticated
            ),
            AccessCase(
                HttpMethod.POST,
                "/v2/auth/me",
                roles(UserRole.USER, UserRole.ORGANIZER, UserRole.ADMIN)
            ),
            AccessCase(
                HttpMethod.GET,
                "/v2/users/lookup",
                roles(UserRole.ORGANIZER, UserRole.ADMIN)
            ),
            AccessCase(HttpMethod.GET, "/v1/admin/ping", roles(UserRole.ADMIN)),
            AccessCase(HttpMethod.GET, "/v1/users/value", AccessRequirement.Authenticated),
            AccessCase(HttpMethod.GET, "/v2/auth/login", AccessRequirement.Authenticated)
        )

        cases.forEach { case ->
            assertEquals(
                case.requirement,
                AuthAccessPolicyCatalog.evaluate(case.method, case.path),
                "${case.method} ${case.path}"
            )
        }
    }

    private fun roles(vararg roles: UserRole): AccessRequirement.AnyRole {
        return AccessRequirement.AnyRole(roles.toSet())
    }

    private fun fingerprint(endpointAccess: List<EndpointAccess>): String {
        val canonical = endpointAccess.joinToString("\n") { endpoint ->
            "${endpoint.method.name()} ${endpoint.path} ${endpoint.requirement.canonicalName()}"
        }
        return sha256(canonical)
    }

    private fun sha256(canonical: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(canonical.toByteArray(StandardCharsets.UTF_8))
        return HexFormat.of().formatHex(digest)
    }

    private fun AccessRequirement.canonicalName(): String {
        return when (this) {
            AccessRequirement.PermitAll -> "PERMIT_ALL"
            AccessRequirement.Authenticated -> "AUTHENTICATED"
            is AccessRequirement.AnyRole -> when (roles) {
                setOf(UserRole.USER, UserRole.ORGANIZER, UserRole.ADMIN) -> "USER_ORGANIZER_ADMIN"
                setOf(UserRole.ORGANIZER, UserRole.ADMIN) -> "ORGANIZER_ADMIN"
                setOf(UserRole.ADMIN) -> "ADMIN"
                else -> error("Unsupported role set: $roles")
            }
        }
    }

    private data class EndpointAccess(
        val method: HttpMethod,
        val path: String,
        val requirement: AccessRequirement
    )

    private data class AccessCase(
        val method: HttpMethod,
        val path: String,
        val requirement: AccessRequirement
    )
}
