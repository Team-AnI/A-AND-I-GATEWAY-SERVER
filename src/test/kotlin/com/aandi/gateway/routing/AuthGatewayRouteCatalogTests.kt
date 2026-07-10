package com.aandi.gateway.routing

import org.junit.jupiter.api.Test
import org.springframework.boot.env.YamlPropertySourceLoader
import org.springframework.core.env.EnumerablePropertySource
import org.springframework.core.io.ClassPathResource
import org.springframework.http.HttpMethod
import java.net.URI
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class AuthGatewayRouteCatalogTests {

    @Test
    fun `auth route catalog is structurally valid`() {
        AuthGatewayRouteCatalog.catalog.validate()
    }

    @Test
    fun `auth route catalog inventory remains stable`() {
        val catalog = AuthGatewayRouteCatalog.catalog
        val routes = catalog.routes

        assertEquals(56, routes.size)
        assertEquals(56, routes.map { it.id }.toSet().size)
        assertEquals(1, catalog.targets.size)
        assertTrue(routes.all { it.target == ServiceTargetKey("AUTH_SERVICE_URI") })
        assertEquals(
            mapOf(
                HttpMethod.POST to 24,
                HttpMethod.GET to 14,
                HttpMethod.PATCH to 10,
                HttpMethod.DELETE to 4,
                null to 4
            ),
            routes.groupingBy { it.method }.eachCount()
        )
        assertEquals(mapOf(0 to 54, -2 to 1, -1 to 1), routes.groupingBy { it.order }.eachCount())
        assertEquals(54, routes.count { it.filters.isEmpty() })
        assertEquals(1, routes.count { it.filters.singleOrNull() is RouteFilterContract.SetPath })
        assertEquals(1, routes.count { it.filters.singleOrNull() is RouteFilterContract.RewritePath })
    }

    @Test
    fun `auth route catalog matches application yaml in declaration order`() {
        assertEquals(AuthGatewayRouteCatalog.catalog, loadAuthCatalogFromYaml())
    }

    @Test
    fun `auth OpenAPI rewrite preserves the trailing path`() {
        val route = AuthGatewayRouteCatalog.catalog.routes.single {
            it.id == RouteId("auth-service-openapi-subpaths")
        }
        val filter = route.filters.single() as RouteFilterContract.RewritePath

        assertEquals(
            "/v3/api-docs/v1",
            Regex(filter.regexp).replace("/v2/auth/v3/api-docs/v1", filter.replacement)
        )
    }

    private fun loadAuthCatalogFromYaml(): GatewayRouteCatalog {
        val propertySource = YamlPropertySourceLoader()
            .load("application", ClassPathResource("application.yaml"))
            .single()
        require(propertySource is EnumerablePropertySource<*>) {
            "application.yaml must expose enumerable properties"
        }

        val targets = linkedMapOf<ServiceTargetKey, URI>()
        val routes = mutableListOf<GatewayRouteContract>()
        var index = 0
        while (true) {
            val prefix = "$routesProperty[$index]"
            val id = propertySource.string("$prefix.id") ?: break
            val rawUri = requireNotNull(propertySource.string("$prefix.uri")) {
                "Missing URI for $id"
            }
            val targetMatch = targetPlaceholder.matchEntire(rawUri)
            if (targetMatch?.groupValues?.get(1) == authTargetKey) {
                val target = ServiceTargetKey(targetMatch.groupValues[1])
                val defaultUri = URI.create(targetMatch.groupValues[2])
                val existingDefault = targets[target]
                require(existingDefault == null || existingDefault == defaultUri) {
                    "Conflicting defaults for ${target.value}: $existingDefault and $defaultUri"
                }
                targets[target] = defaultUri

                val predicates = propertySource.indexedStrings("$prefix.predicates")
                val pathValues = predicates.filter { it.startsWith("Path=") }.map { it.removePrefix("Path=") }
                val methodValues = predicates.filter { it.startsWith("Method=") }.map { it.removePrefix("Method=") }
                require(pathValues.size == 1 && methodValues.size <= 1) {
                    "Route $id must have exactly one Path and at most one Method predicate: $predicates"
                }
                val method = methodValues.singleOrNull()?.let(HttpMethod::valueOf)
                val expectedPredicates = buildList {
                    add("Path=${pathValues.single()}")
                    method?.let { add("Method=$it") }
                }
                require(predicates == expectedPredicates) {
                    "Route $id predicate order or type changed: $predicates"
                }

                routes += GatewayRouteContract(
                    id = RouteId(id),
                    target = target,
                    path = RoutePathPattern(pathValues.single()),
                    method = method,
                    order = propertySource.int("$prefix.order") ?: 0,
                    filters = propertySource.indexedStrings("$prefix.filters").map(::parseFilter),
                    enabled = propertySource.boolean("$prefix.enabled") ?: true,
                    metadata = propertySource.metadata("$prefix.metadata")
                )
            }
            index++
        }

        return GatewayRouteCatalog(targets = targets, routes = routes)
    }

    private fun parseFilter(raw: String): RouteFilterContract {
        return when {
            raw.startsWith("SetPath=") -> RouteFilterContract.SetPath(raw.removePrefix("SetPath="))
            raw.startsWith("RewritePath=") -> {
                val arguments = raw.removePrefix("RewritePath=").split(",", limit = 2).map(String::trim)
                require(arguments.size == 2) { "Invalid RewritePath filter: $raw" }
                RouteFilterContract.RewritePath(
                    regexp = arguments[0],
                    replacement = arguments[1].replace("\$\\{", "\${")
                )
            }

            else -> error("Unsupported route filter: $raw")
        }
    }

    private fun EnumerablePropertySource<*>.indexedStrings(prefix: String): List<String> {
        val values = mutableListOf<String>()
        var index = 0
        while (true) {
            val value = string("$prefix[$index]") ?: break
            values += value
            index++
        }
        return values
    }

    private fun EnumerablePropertySource<*>.metadata(prefix: String): Map<String, Any> {
        val nestedPrefix = "$prefix."
        return propertyNames
            .filter { it.startsWith(nestedPrefix) }
            .associate { name -> name.removePrefix(nestedPrefix) to requireNotNull(getProperty(name)) }
    }

    private fun EnumerablePropertySource<*>.string(name: String): String? {
        return getProperty(name)?.toString()
    }

    private fun EnumerablePropertySource<*>.int(name: String): Int? {
        return getProperty(name)?.toString()?.toInt()
    }

    private fun EnumerablePropertySource<*>.boolean(name: String): Boolean? {
        return getProperty(name)?.toString()?.toBooleanStrict()
    }

    private companion object {
        private const val authTargetKey = "AUTH_SERVICE_URI"
        private const val routesProperty = "spring.cloud.gateway.server.webflux.routes"
        private val targetPlaceholder = Regex("""\$\{([A-Z][A-Z0-9_]*):([^}]+)}""")
    }
}
