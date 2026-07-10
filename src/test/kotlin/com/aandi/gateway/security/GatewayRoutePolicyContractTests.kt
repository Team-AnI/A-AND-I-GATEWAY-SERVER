package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.cloud.gateway.route.Route
import org.springframework.cloud.gateway.route.RouteDefinition
import org.springframework.cloud.gateway.route.RouteDefinitionLocator
import org.springframework.cloud.gateway.route.RouteLocator
import org.springframework.http.HttpMethod
import org.springframework.http.MediaType
import org.springframework.mock.http.server.reactive.MockServerHttpRequest
import org.springframework.mock.web.server.MockServerWebExchange
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Flux
import reactor.core.publisher.Mono
import java.net.URI
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.test.assertEquals
import kotlin.test.assertTrue

@SpringBootTest(
    properties = [
        "POST_SERVICE_URI=http://localhost:8084",
        "AUTH_SERVICE_URI=http://localhost:9000",
        "ONLINE_JUDGE_SERVICE_URI=http://localhost:8080",
        "app.security.internal-event-token=test-internal-token",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!",
        "app.security.policy.enforce-https=false",
        "app.security.policy.max-request-body-size=1KB"
    ]
)
class GatewayRoutePolicyContractTests(
    @Autowired private val routeDefinitionLocator: RouteDefinitionLocator,
    @Autowired private val routeLocator: RouteLocator,
    @Autowired private val requestPolicyFilter: GatewayRequestPolicyFilter
) {

    @Test
    fun `route inventory remains stable`() {
        val definitions = routeDefinitions()
        val routes = orderedRoutes()
        val definitionIds = definitions.map(::routeId)
        val routeIds = routes.map { it.id }

        assertEquals(141, definitions.size)
        assertEquals(141, definitionIds.toSet().size, "route definition IDs must be unique")
        assertEquals(141, routes.size)
        assertEquals(141, routeIds.toSet().size, "compiled route IDs must be unique")
        assertEquals(definitionIds.toSet(), routeIds.toSet(), "compiled routes must match route definitions")
        assertEquals(
            mapOf("AUTH" to 56, "REPORT" to 21, "POST" to 44, "ONLINE_JUDGE" to 20),
            definitions.groupingBy { serviceName(routeId(it)) }.eachCount()
        )
        assertEquals(118, definitions.count { declaredMethods(it).isNotEmpty() })
        assertEquals(23, definitions.count { declaredMethods(it).isEmpty() })
    }

    @Test
    fun `openapi routes remain get only with deterministic precedence`() {
        val definitions = routeDefinitions().associateBy(::routeId)

        listOf("post", "report", "auth", "online-judge").forEach { service ->
            val root = requireNotNull(definitions["$service-service-openapi-root"])
            val subpaths = requireNotNull(definitions["$service-service-openapi-subpaths"])

            assertEquals(listOf(HttpMethod.GET), declaredMethods(root))
            assertEquals(listOf(HttpMethod.GET), declaredMethods(subpaths))
            assertEquals(-2, root.order)
            assertEquals(-1, subpaths.order)
            assertTrue(root.order < subpaths.order)
        }
    }

    @Test
    fun `report post aliases remain ahead of post catch all`() {
        val definitions = routeDefinitions().associateBy(::routeId)
        val catchAll = requireNotNull(definitions["post-service-posts-subpaths"])
        val expectedOrders = mapOf(
            "report-service-admin-courses-root" to -2,
            "report-service-admin-courses-subpaths" to -1,
            "report-service-courses-root" to -2,
            "report-service-courses-subpaths" to -1
        )

        assertEquals(0, catchAll.order)
        expectedOrders.forEach { (routeId, expectedOrder) ->
            val route = requireNotNull(definitions[routeId])
            assertEquals(expectedOrder, route.order)
            assertTrue(route.order < catchAll.order)
        }
    }

    @Test
    fun `routes with method predicates remain reachable through request policy`() {
        val routes = orderedRoutes()
        val failures = routeDefinitions()
            .map { it to declaredMethods(it) }
            .filter { (_, methods) -> methods.isNotEmpty() }
            .flatMap { (definition, methods) ->
                methods.mapNotNull { method ->
                    val routeId = routeId(definition)
                    val evaluations = pathWitnesses(definition)
                        .map { path -> evaluate(routes, Candidate(method, path)) }

                    if (evaluations.any { it.reaches(routeId) }) {
                        null
                    } else {
                        "$routeId [$method] ${evaluations.joinToString()}"
                    }
                }
            }

        assertTrue(
            failures.isEmpty(),
            failures.joinToString(
                prefix = "Routes with explicit methods must have a policy-allowed witness selected as that route:\n",
                separator = "\n"
            )
        )
    }

    @Test
    fun `routes without method predicates retain at least one policy allowed method`() {
        val routes = orderedRoutes()
        val failures = routeDefinitions()
            .filter { declaredMethods(it).isEmpty() }
            .mapNotNull { definition ->
                val routeId = routeId(definition)
                val evaluations = pathWitnesses(definition).flatMap { path ->
                    supportedMethods.map { method -> evaluate(routes, Candidate(method, path)) }
                }

                if (evaluations.any { it.reaches(routeId) }) {
                    null
                } else {
                    "$routeId ${evaluations.joinToString()}"
                }
            }

        assertTrue(
            failures.isEmpty(),
            failures.joinToString(
                prefix = "Routes without Method predicates must retain a non-OPTIONS policy-allowed witness:\n",
                separator = "\n"
            )
        )
    }

    private fun routeDefinitions(): List<RouteDefinition> {
        return routeDefinitionLocator.routeDefinitions.collectList().block().orEmpty()
    }

    private fun orderedRoutes(): List<Route> {
        return routeLocator.routes.collectList().block().orEmpty()
    }

    private fun routeId(definition: RouteDefinition): String {
        return requireNotNull(definition.id) { "Route definition ID must not be null" }
    }

    private fun declaredMethods(definition: RouteDefinition): List<HttpMethod> {
        val predicates = definition.predicates.filter { it.name == "Method" }
        require(predicates.size <= 1) {
            "Route ${definition.id} must not declare multiple Method predicates"
        }

        return predicates.singleOrNull()
            ?.args
            ?.values
            .orEmpty()
            .flatMap { it.split(",") }
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .map { HttpMethod.valueOf(it) }
    }

    private fun pathWitnesses(definition: RouteDefinition): List<String> {
        val predicates = definition.predicates.filter { it.name == "Path" }
        require(predicates.size == 1) {
            "Route ${definition.id} must declare exactly one Path predicate"
        }

        val patterns = predicates.single().args.values
            .map { it.trim() }
            .filter { it.startsWith("/") }
        require(patterns.isNotEmpty()) {
            "Route ${definition.id} must declare at least one path pattern"
        }

        return patterns.flatMap(::materializePathWitnesses).distinct()
    }

    private fun materializePathWitnesses(pattern: String): List<String> {
        val materialized = pattern.replace(pathVariablePattern, "sample")
        if (!materialized.endsWith("/**")) {
            return listOf(materialized.replace("*", "sample"))
        }

        val basePath = materialized.removeSuffix("/**")
            .replace("*", "sample")
            .ifEmpty { "/" }

        return listOf(
            basePath,
            appendPath(basePath, "sample"),
            appendPath(basePath, "sample/nested")
        ).distinct()
    }

    private fun appendPath(basePath: String, suffix: String): String {
        return if (basePath == "/") "/$suffix" else "$basePath/$suffix"
    }

    private fun evaluate(routes: List<Route>, candidate: Candidate): Evaluation {
        return Evaluation(
            candidate = candidate,
            policyAllowed = isPolicyAllowed(candidate),
            selectedRouteId = firstMatchingRouteId(routes, candidate)
        )
    }

    private fun isPolicyAllowed(candidate: Candidate): Boolean {
        val chainReached = AtomicBoolean(false)
        requestPolicyFilter.filter(
            exchange(candidate),
            WebFilterChain {
                chainReached.set(true)
                Mono.empty()
            }
        ).block()
        return chainReached.get()
    }

    private fun firstMatchingRouteId(routes: List<Route>, candidate: Candidate): String? {
        val exchange = exchange(candidate)
        return Flux.fromIterable(routes)
            .filterWhen { route -> route.predicate.apply(exchange) }
            .next()
            .map { it.id }
            .block()
    }

    private fun exchange(candidate: Candidate): ServerWebExchange {
        val request = MockServerHttpRequest.method(
            candidate.method,
            URI.create("http://localhost${candidate.path}")
        )
            .contentType(MediaType.APPLICATION_JSON)
            .header("X-Forwarded-Proto", "https")
            .build()
        return MockServerWebExchange.from(request)
    }

    private fun serviceName(routeId: String): String {
        return when {
            routeId.startsWith("auth-service") -> "AUTH"
            routeId.startsWith("report-service") -> "REPORT"
            routeId.startsWith("post-service") -> "POST"
            routeId.startsWith("online-judge-service") -> "ONLINE_JUDGE"
            else -> error("Unknown service route ID: $routeId")
        }
    }

    private data class Candidate(
        val method: HttpMethod,
        val path: String
    )

    private data class Evaluation(
        val candidate: Candidate,
        val policyAllowed: Boolean,
        val selectedRouteId: String?
    ) {
        fun reaches(expectedRouteId: String): Boolean {
            return policyAllowed && selectedRouteId == expectedRouteId
        }

        override fun toString(): String {
            return "${candidate.method} ${candidate.path} " +
                "(policyAllowed=$policyAllowed, selectedRoute=$selectedRouteId)"
        }
    }

    private companion object {
        private val pathVariablePattern = Regex("""\{[^/{}]+}""")
        private val supportedMethods = listOf(
            HttpMethod.GET,
            HttpMethod.POST,
            HttpMethod.PUT,
            HttpMethod.PATCH,
            HttpMethod.DELETE
        )
    }
}
