package com.aandi.gateway.security

import com.aandi.gateway.common.response.GatewayResponseWriter
import org.junit.jupiter.api.Test
import org.springframework.http.HttpMethod
import org.springframework.http.HttpStatus
import org.springframework.http.server.PathContainer
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono
import tools.jackson.databind.ObjectMapper
import java.net.URI
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.util.HexFormat
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class MethodPathPolicyEvaluatorTests {

    private val responseWriter = GatewayResponseWriter(ObjectMapper(), "https://*")

    @Test
    fun `deny wins and wildcard method path semantics remain stable`() {
        val evaluator = MethodPathPolicyEvaluator(
            allowRules = listOf(AllowRule(HttpMethod.GET, "/v2/items/**")),
            denyRules = listOf(AllowRule(HttpMethod.GET, "/v2/items/private/**"))
        )

        listOf("/v2/items", "/v2/items/a/b").forEach { path ->
            assertEquals(MethodPathDecision.ALLOW, evaluator.evaluate(HttpMethod.GET, path(path)), path)
        }
        listOf("/v2/items/private", "/v2/items/private/a/b").forEach { path ->
            assertEquals(MethodPathDecision.EXPLICIT_DENY, evaluator.evaluate(HttpMethod.GET, path(path)), path)
        }
        assertEquals(MethodPathDecision.NO_MATCH, evaluator.evaluate(null, path("/v2/items")))
        assertEquals(MethodPathDecision.NO_MATCH, evaluator.evaluate(HttpMethod.POST, path("/v2/items")))
        assertEquals(MethodPathDecision.NO_MATCH, evaluator.evaluate(HttpMethod.GET, path("/other")))
    }

    @Test
    fun `current method path policy inventory remains stable and duplicate free`() {
        val evaluator = filter().methodPathPolicy
        val allowKeys = evaluator.allowRules.map { it.method to it.path }
        val denyKeys = evaluator.denyRules.map { it.method to it.path }

        assertEquals(254, allowKeys.size)
        assertEquals(254, allowKeys.toSet().size)
        assertEquals(13, denyKeys.size)
        assertEquals(13, denyKeys.toSet().size)
        assertEquals(
            mapOf(
                HttpMethod.GET to 126,
                HttpMethod.POST to 60,
                HttpMethod.PATCH to 32,
                HttpMethod.DELETE to 27,
                HttpMethod.PUT to 9
            ),
            evaluator.allowRules.groupingBy { it.method }.eachCount()
        )
        assertEquals(
            mapOf(
                HttpMethod.GET to 1,
                HttpMethod.POST to 2,
                HttpMethod.PUT to 2,
                HttpMethod.PATCH to 4,
                HttpMethod.DELETE to 4
            ),
            evaluator.denyRules.groupingBy { it.method }.eachCount()
        )
        assertEquals(
            setOf(
                HttpMethod.GET to "/v2/admin/courses/{courseSlug}/assignments/copy",
                HttpMethod.PATCH to "/v2/admin/courses/{courseSlug}/assignments/copy",
                HttpMethod.DELETE to "/v2/admin/courses/{courseSlug}/assignments/copy",
                HttpMethod.PATCH to "/v2/post/courses",
                HttpMethod.DELETE to "/v2/post/courses",
                HttpMethod.POST to "/v2/report/v3/api-docs",
                HttpMethod.POST to "/v2/report/v3/api-docs/**",
                HttpMethod.PUT to "/v2/report/v3/api-docs",
                HttpMethod.PUT to "/v2/report/v3/api-docs/**",
                HttpMethod.PATCH to "/v2/report/v3/api-docs",
                HttpMethod.PATCH to "/v2/report/v3/api-docs/**",
                HttpMethod.DELETE to "/v2/report/v3/api-docs",
                HttpMethod.DELETE to "/v2/report/v3/api-docs/**"
            ),
            denyKeys.toSet()
        )
    }

    @Test
    fun `auth endpoint policy catalog preserves the ordered allow inventory partition`() {
        val allowKeys = filter().methodPathPolicy.allowRules.map { it.method to it.path }
        val authKeys = AuthEndpointPolicyCatalog.allowRules.map { it.method to it.path }
        val authKeySet = authKeys.toSet()
        val remainingKeys = allowKeys.filterNot(authKeySet::contains)

        assertEquals(28, AuthEndpointPolicyCatalog.legacyRules.size)
        assertEquals(2, AuthEndpointPolicyCatalog.pingRules.size)
        assertEquals(2, AuthEndpointPolicyCatalog.openApiRules.size)
        assertEquals(41, AuthEndpointPolicyCatalog.v2Rules.size)
        assertEquals(73, authKeys.size)
        assertEquals(73, authKeySet.size)
        assertEquals(181, remainingKeys.size)
        assertEquals(181, remainingKeys.toSet().size)
        assertEquals(254, allowKeys.size)
        assertEquals(emptySet(), authKeySet intersect remainingKeys.toSet())
        assertEquals(authKeys, allowKeys.filter(authKeySet::contains))
        assertEquals(
            mapOf(
                HttpMethod.GET to 18,
                HttpMethod.POST to 28,
                HttpMethod.PATCH to 14,
                HttpMethod.DELETE to 9,
                HttpMethod.PUT to 4
            ),
            AuthEndpointPolicyCatalog.allowRules.groupingBy { it.method }.eachCount()
        )
        assertEquals(
            "3faa56cb0e6d163e35b5944f296c8bd9ea99f6346c30ac4abd4f3701dda469db",
            fingerprint(allowKeys)
        )
        assertEquals(
            "4820cf7292ceb76c825ce8671274386bdba8d0757e7a28e6798a5d151bd9e18b",
            fingerprint(authKeys)
        )
        assertEquals(
            "972c97e3cbf7e74eeecc3b7abf755c46a7bac62832d2b2f3b2436c42c8e310c1",
            fingerprint(remainingKeys)
        )

        setOf(
            HttpMethod.POST to "/activate",
            HttpMethod.POST to "/v1/admin/invite-mail",
            HttpMethod.DELETE to "/v1/users/**",
            HttpMethod.GET to "/v2/ping/**",
            HttpMethod.GET to "/v2/auth/v3/api-docs/**",
            HttpMethod.PATCH to "/v2/me/password",
            HttpMethod.GET to "/v2/users/lookup",
            HttpMethod.GET to "/v2/admin/ping"
        ).forEach { key -> assertTrue(key in authKeySet, key.toString()) }

        setOf(
            HttpMethod.GET to "/api/ping/**",
            HttpMethod.GET to "/v3/api-docs",
            HttpMethod.POST to "/internal/v1/cache/invalidation",
            HttpMethod.GET to "/v1/admin/courses",
            HttpMethod.GET to "/v2/admin/courses",
            HttpMethod.GET to "/v2/admin/submissions",
            HttpMethod.GET to "/v2/post"
        ).forEach { key -> assertFalse(key in authKeySet, key.toString()) }
    }

    @Test
    fun `auth catalog extraction keeps method path decisions unchanged`() {
        val evaluator = filter().methodPathPolicy
        val cases = listOf(
            DecisionCase(HttpMethod.POST, "/v1/auth/login", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.POST, "/activate", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.GET, "/v1/users/user-1", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.GET, "/v2/ping", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.GET, "/v2/ping/deep/path", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.GET, "/v2/auth/v3/api-docs/users", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.PATCH, "/v2/me/password", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.DELETE, "/v2/admin/users/user-1", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.GET, "/v1/auth/login", MethodPathDecision.NO_MATCH),
            DecisionCase(HttpMethod.POST, "/v2/ping", MethodPathDecision.NO_MATCH),
            DecisionCase(HttpMethod.POST, "/v2/auth/v3/api-docs", MethodPathDecision.NO_MATCH),
            DecisionCase(null, "/v2/me", MethodPathDecision.NO_MATCH),
            DecisionCase(HttpMethod.GET, "/api/ping/service", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.POST, "/internal/v1/cache/invalidation", MethodPathDecision.ALLOW),
            DecisionCase(HttpMethod.GET, "/v2/post", MethodPathDecision.ALLOW),
            DecisionCase(
                HttpMethod.GET,
                "/v2/admin/courses/course/assignments/copy",
                MethodPathDecision.EXPLICIT_DENY
            ),
            DecisionCase(HttpMethod.GET, "/not-allowlisted", MethodPathDecision.NO_MATCH)
        )

        cases.forEach { case ->
            assertEquals(
                case.expected,
                evaluator.evaluate(case.method, path(case.path)),
                "${case.method} ${case.path}"
            )
        }
    }

    @Test
    fun `report endpoint policy catalog preserves the ordered allow inventory partition`() {
        val allowKeys = filter().methodPathPolicy.allowRules.map { it.method to it.path }
        val reportKeys = ReportEndpointPolicyCatalog.allowRules.map { it.method to it.path }
        val reportKeySet = reportKeys.toSet()

        assertEquals(2, ReportEndpointPolicyCatalog.openApiRules.size)
        assertEquals(14, ReportEndpointPolicyCatalog.serviceRules.size)
        assertEquals(16, reportKeys.size)
        assertEquals(16, reportKeySet.size)
        assertEquals(reportKeys, allowKeys.filter(reportKeySet::contains))
        assertEquals(
            mapOf(
                HttpMethod.GET to 6,
                HttpMethod.POST to 4,
                HttpMethod.PUT to 2,
                HttpMethod.PATCH to 2,
                HttpMethod.DELETE to 2
            ),
            ReportEndpointPolicyCatalog.allowRules.groupingBy { it.method }.eachCount()
        )
        assertEquals(
            "3faa56cb0e6d163e35b5944f296c8bd9ea99f6346c30ac4abd4f3701dda469db",
            fingerprint(allowKeys)
        )
    }

    @Test
    fun `filter maps evaluator decisions without changing endpoint error contract`() {
        val filter = filter()

        val allowed = evaluateFilter(filter, "/v2/ping")
        assertTrue(allowed.chainInvoked)
        assertNull(allowed.status)

        listOf(
            "/v2/admin/courses/course/assignments/copy",
            "/not-allowlisted"
        ).forEach { path ->
            val rejected = evaluateFilter(filter, path)

            assertFalse(rejected.chainInvoked, path)
            assertEquals(HttpStatus.NOT_FOUND, rejected.status, path)
            assertTrue(rejected.body.contains("\"code\":15001"), path)
            assertTrue(rejected.body.contains("\"value\":\"ENDPOINT_NOT_ALLOWLISTED\""), path)
        }
    }

    private fun filter(): GatewayRequestPolicyFilter {
        return GatewayRequestPolicyFilter(
            policy = SecurityPolicyProperties(enforceJsonContentType = false),
            responseWriter = responseWriter
        )
    }

    private fun evaluateFilter(filter: GatewayRequestPolicyFilter, path: String): FilterResult {
        val request = MockServerHttpRequest.method(
            HttpMethod.GET,
            URI.create("http://localhost$path")
        ).build()
        val exchange = MockServerWebExchange.from(request)
        var chainInvoked = false
        val chain = WebFilterChain {
            chainInvoked = true
            Mono.empty()
        }

        filter.filter(exchange, chain).block()

        return FilterResult(
            chainInvoked = chainInvoked,
            status = exchange.response.statusCode,
            body = if (exchange.response.statusCode == null) {
                ""
            } else {
                exchange.response.bodyAsString.block().orEmpty()
            }
        )
    }

    private fun path(value: String): PathContainer = PathContainer.parsePath(value)

    private fun fingerprint(keys: List<Pair<HttpMethod, String>>): String {
        val canonical = keys.joinToString("\n") { (method, path) -> "${method.name()} $path" }
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(canonical.toByteArray(StandardCharsets.UTF_8))
        return HexFormat.of().formatHex(digest)
    }

    private data class DecisionCase(
        val method: HttpMethod?,
        val path: String,
        val expected: MethodPathDecision
    )

    private data class FilterResult(
        val chainInvoked: Boolean,
        val status: org.springframework.http.HttpStatusCode?,
        val body: String
    )
}
