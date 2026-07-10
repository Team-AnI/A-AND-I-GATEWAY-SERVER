package com.aandi.gateway.routing

import org.springframework.http.HttpMethod
import java.net.URI

internal object AuthGatewayRouteCatalog {
    private val authTarget = ServiceTargetKey("AUTH_SERVICE_URI")

    val catalog = GatewayRouteCatalog(
        targets = mapOf(authTarget to URI.create("http://localhost:9000")),
        routes = listOf(
            authRoute("auth-service-v1-login", "/v1/auth/login", HttpMethod.POST),
            authRoute("auth-service-v1-refresh", "/v1/auth/refresh", HttpMethod.POST),
            authRoute("auth-service-v1-logout", "/v1/auth/logout", HttpMethod.POST),
            authRoute("auth-service-v1-me", "/v1/me", HttpMethod.GET),
            authRoute("auth-service-v1-me-post", "/v1/me", HttpMethod.POST),
            authRoute("auth-service-v1-me-patch", "/v1/me", HttpMethod.PATCH),
            authRoute("auth-service-v1-me-password", "/v1/me/password", HttpMethod.POST),
            authRoute("auth-service-v1-admin-ping", "/v1/admin/ping", HttpMethod.GET),
            authRoute("auth-service-v1-admin-users-get", "/v1/admin/users", HttpMethod.GET),
            authRoute("auth-service-v1-admin-users-post", "/v1/admin/users", HttpMethod.POST),
            authRoute("auth-service-v1-admin-users-sync-post", "/v1/admin/users/sync", HttpMethod.POST),
            authRoute("auth-service-v1-admin-invite-mail-post", "/v1/admin/invite-mail", HttpMethod.POST),
            authRoute("auth-service-v1-admin-users-role-patch", "/v1/admin/users/role", HttpMethod.PATCH),
            authRoute("auth-service-v1-admin-users-patch", "/v1/admin/users/**", HttpMethod.PATCH),
            authRoute(
                "auth-service-v1-admin-users-reset-password",
                "/v1/admin/users/*/reset-password",
                HttpMethod.POST
            ),
            authRoute("auth-service-v1-admin-users-delete", "/v1/admin/users/**", HttpMethod.DELETE),
            authRoute("auth-service-v1-users-root", "/v1/users"),
            authRoute("auth-service-v1-users-subpaths", "/v1/users/**"),
            authRoute("auth-service-login", "/v2/auth/login", HttpMethod.POST),
            authRoute("auth-service-refresh", "/v2/auth/refresh", HttpMethod.POST),
            authRoute("auth-service-logout", "/v2/auth/logout", HttpMethod.POST),
            authRoute("auth-service-activate", "/activate", HttpMethod.POST),
            authRoute("auth-service-v2-activate", "/v2/activate", HttpMethod.POST),
            authRoute("auth-service-v2-ping", "/v2/ping", HttpMethod.GET),
            authRoute("auth-service-v2-ping-subpaths", "/v2/ping/**", HttpMethod.GET),
            authRoute("auth-service-me", "/v2/auth/me", HttpMethod.GET),
            authRoute("auth-service-me-post", "/v2/auth/me", HttpMethod.POST),
            authRoute("auth-service-me-patch", "/v2/auth/me", HttpMethod.PATCH),
            authRoute("auth-service-me-password-post", "/v2/auth/me/password", HttpMethod.POST),
            authRoute("auth-service-admin-ping", "/v2/auth/admin/ping", HttpMethod.GET),
            authRoute("auth-service-admin-users-get", "/v2/auth/admin/users", HttpMethod.GET),
            authRoute("auth-service-admin-users-post", "/v2/auth/admin/users", HttpMethod.POST),
            authRoute("auth-service-admin-users-sync-post", "/v2/auth/admin/users/sync", HttpMethod.POST),
            authRoute("auth-service-admin-invite-mail-post", "/v2/auth/admin/invite-mail", HttpMethod.POST),
            authRoute("auth-service-admin-users-role-patch", "/v2/auth/admin/users/role", HttpMethod.PATCH),
            authRoute("auth-service-admin-users-patch", "/v2/auth/admin/users/**", HttpMethod.PATCH),
            authRoute(
                "auth-service-admin-users-reset-password",
                "/v2/auth/admin/users/*/reset-password",
                HttpMethod.POST
            ),
            authRoute("auth-service-admin-users-delete-root", "/v2/auth/admin/users", HttpMethod.DELETE),
            authRoute("auth-service-admin-users-delete", "/v2/auth/admin/users/**", HttpMethod.DELETE),
            authRoute("auth-service-users-root", "/v2/auth/users"),
            authRoute("auth-service-users-subpaths", "/v2/auth/users/**"),
            authRoute("auth-service-v2-me", "/v2/me", HttpMethod.GET),
            authRoute("auth-service-v2-me-patch", "/v2/me", HttpMethod.PATCH),
            authRoute("auth-service-v2-me-upload-url", "/v2/me/profile-image/upload-url", HttpMethod.POST),
            authRoute("auth-service-v2-me-password", "/v2/me/password", HttpMethod.PATCH),
            authRoute("auth-service-v2-users-lookup", "/v2/users/lookup", HttpMethod.GET),
            authRoute("auth-service-v2-admin-ping", "/v2/admin/ping", HttpMethod.GET),
            authRoute("auth-service-v2-admin-users-get", "/v2/admin/users", HttpMethod.GET),
            authRoute("auth-service-v2-admin-users-post", "/v2/admin/users", HttpMethod.POST),
            authRoute("auth-service-v2-admin-invite-mail-post", "/v2/admin/invite-mail", HttpMethod.POST),
            authRoute(
                "auth-service-v2-admin-users-reset-password",
                "/v2/admin/users/*/password/reset",
                HttpMethod.POST
            ),
            authRoute("auth-service-v2-admin-users-role-patch", "/v2/admin/users/*/role", HttpMethod.PATCH),
            authRoute("auth-service-v2-admin-users-patch", "/v2/admin/users/*", HttpMethod.PATCH),
            authRoute("auth-service-v2-admin-users-delete", "/v2/admin/users/*", HttpMethod.DELETE),
            authRoute(
                id = "auth-service-openapi-root",
                path = "/v2/auth/v3/api-docs",
                method = HttpMethod.GET,
                order = -2,
                filters = listOf(RouteFilterContract.SetPath("/v3/api-docs"))
            ),
            authRoute(
                id = "auth-service-openapi-subpaths",
                path = "/v2/auth/v3/api-docs/**",
                method = HttpMethod.GET,
                order = -1,
                filters = listOf(
                    RouteFilterContract.RewritePath(
                        regexp = "/v2/auth/v3/api-docs/(?<segment>.*)",
                        replacement = "/v3/api-docs/\${segment}"
                    )
                )
            )
        )
    )

    private fun authRoute(
        id: String,
        path: String,
        method: HttpMethod? = null,
        order: Int = 0,
        filters: List<RouteFilterContract> = emptyList()
    ): GatewayRouteContract {
        return GatewayRouteContract(
            id = RouteId(id),
            target = authTarget,
            path = RoutePathPattern(path),
            method = method,
            order = order,
            filters = filters
        )
    }
}
