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

    private data class FilterResult(
        val chainInvoked: Boolean,
        val status: org.springframework.http.HttpStatusCode?,
        val body: String
    )
}
