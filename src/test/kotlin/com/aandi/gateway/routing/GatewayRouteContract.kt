package com.aandi.gateway.routing

import org.springframework.http.HttpMethod
import org.springframework.web.util.pattern.PathPatternParser
import java.net.URI
import java.util.regex.Pattern

@JvmInline
internal value class RouteId(val value: String)

@JvmInline
internal value class ServiceTargetKey(val value: String)

@JvmInline
internal value class RoutePathPattern(val value: String)

internal data class GatewayRouteCatalog(
    val targets: Map<ServiceTargetKey, URI>,
    val routes: List<GatewayRouteContract>
)

internal data class GatewayRouteContract(
    val id: RouteId,
    val target: ServiceTargetKey,
    val path: RoutePathPattern,
    val method: HttpMethod? = null,
    val order: Int = 0,
    val filters: List<RouteFilterContract> = emptyList(),
    val enabled: Boolean = true,
    val metadata: Map<String, Any> = emptyMap()
)

internal sealed interface RouteFilterContract {
    data class SetPath(val path: String) : RouteFilterContract

    data class RewritePath(
        val regexp: String,
        val replacement: String
    ) : RouteFilterContract
}

internal fun GatewayRouteCatalog.validate() {
    require(routes.isNotEmpty()) { "Route catalog must not be empty" }
    require(routes.map { it.id }.toSet().size == routes.size) { "Route IDs must be unique" }

    targets.forEach { (key, uri) ->
        require(key.value.matches(Regex("[A-Z][A-Z0-9_]*"))) {
            "Invalid service target key: ${key.value}"
        }
        require(uri.isAbsolute && uri.scheme in setOf("http", "https") && uri.host != null) {
            "Service target must be an absolute HTTP(S) URI: $uri"
        }
    }

    routes.forEach { route ->
        require(route.id.value.isNotBlank()) { "Route ID must not be blank" }
        require(route.target in targets) { "Unknown target ${route.target.value} for ${route.id.value}" }
        PathPatternParser.defaultInstance.parse(route.path.value)

        route.filters.forEach { filter ->
            when (filter) {
                is RouteFilterContract.SetPath -> require(filter.path.startsWith("/")) {
                    "SetPath must start with '/': ${route.id.value}"
                }

                is RouteFilterContract.RewritePath -> {
                    Pattern.compile(filter.regexp)
                    require(filter.replacement.startsWith("/")) {
                        "RewritePath replacement must start with '/': ${route.id.value}"
                    }
                }
            }
        }
    }
}
