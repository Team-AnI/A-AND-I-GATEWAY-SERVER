package com.aandi.gateway.security

import org.springframework.http.HttpMethod
import org.springframework.web.util.pattern.PathPatternParser

internal sealed interface AccessRequirement {
    data object PermitAll : AccessRequirement

    data object Authenticated : AccessRequirement

    data class AnyRole(val roles: Set<UserRole>) : AccessRequirement {
        init {
            require(roles.isNotEmpty()) { "Access roles must not be empty" }
        }
    }
}

internal sealed interface AccessMatcherContract {
    data class Paths(
        val method: HttpMethod?,
        val paths: List<String>
    ) : AccessMatcherContract {
        init {
            require(paths.isNotEmpty()) { "Access paths must not be empty" }
            require(paths.none(String::isBlank)) { "Access paths must not be blank" }
            require(paths.size == paths.toSet().size) { "Access paths must not contain duplicates" }
        }
    }

    data object AnyExchange : AccessMatcherContract
}

internal data class AccessRuleContract(
    val id: String,
    val matcher: AccessMatcherContract,
    val requirement: AccessRequirement
)

/** Ordered source of truth for the authenticated Spring Security chain. */
internal object GlobalAccessPolicyCatalog {
    private val userOrganizerAdmin = AccessRequirement.AnyRole(
        setOf(UserRole.USER, UserRole.ORGANIZER, UserRole.ADMIN)
    )
    private val organizerAdmin = AccessRequirement.AnyRole(setOf(UserRole.ORGANIZER, UserRole.ADMIN))
    private val admin = AccessRequirement.AnyRole(setOf(UserRole.ADMIN))

    val rules: List<AccessRuleContract> = listOf(
        paths("preflight-options", HttpMethod.OPTIONS, AccessRequirement.PermitAll, "/**"),
        paths("public-v1-auth-post", HttpMethod.POST, AccessRequirement.PermitAll, "/v1/auth/**"),
        paths(
            "public-v2-auth-session-post",
            HttpMethod.POST,
            AccessRequirement.PermitAll,
            "/v2/auth/login",
            "/v2/auth/refresh",
            "/v2/auth/logout",
            "/activate",
            "/v2/activate"
        ),
        paths(
            "public-cache-invalidation-post",
            HttpMethod.POST,
            AccessRequirement.PermitAll,
            "/internal/v1/cache/invalidation"
        ),
        paths(
            "public-ping-get",
            HttpMethod.GET,
            AccessRequirement.PermitAll,
            "/api/ping/**",
            "/v2/ping/**"
        ),
        paths("public-index-get", HttpMethod.GET, AccessRequirement.PermitAll, "/", "/index.html"),
        paths(
            "public-root-openapi-get",
            HttpMethod.GET,
            AccessRequirement.PermitAll,
            "/v3/api-docs",
            "/v3/api-docs/**"
        ),
        paths(
            "public-service-openapi-get",
            HttpMethod.GET,
            AccessRequirement.PermitAll,
            "/v2/*/v3/api-docs",
            "/v2/*/v3/api-docs/**"
        ),
        paths(
            "public-swagger-get",
            HttpMethod.GET,
            AccessRequirement.PermitAll,
            "/swagger-ui.html",
            "/swagger-ui/**",
            "/v2/docs",
            "/v2/docs/**",
            "/v2/swagger-ui/index.html",
            "/v2/swagger-ui/**"
        ),
        paths(
            "public-health-get",
            HttpMethod.GET,
            AccessRequirement.PermitAll,
            "/actuator/health",
            "/actuator/health/**"
        ),
        paths("auth-me-get", HttpMethod.GET, userOrganizerAdmin, "/v1/me", "/v2/auth/me", "/v2/me"),
        paths("auth-me-post", HttpMethod.POST, userOrganizerAdmin, "/v1/me", "/v2/auth/me"),
        paths("auth-me-patch", HttpMethod.PATCH, userOrganizerAdmin, "/v1/me", "/v2/auth/me", "/v2/me"),
        paths(
            "auth-profile-image-upload-url-post",
            HttpMethod.POST,
            userOrganizerAdmin,
            "/v2/me/profile-image/upload-url"
        ),
        paths("auth-v1-password-post", HttpMethod.POST, userOrganizerAdmin, "/v1/me/password"),
        paths("auth-v2-password-patch", HttpMethod.PATCH, userOrganizerAdmin, "/v2/me/password"),
        paths("admin-v1-courses-get", HttpMethod.GET, admin, "/v1/admin/courses"),
        paths("organizer-v2-user-lookup-get", HttpMethod.GET, organizerAdmin, "/v2/users/lookup"),
        paths(
            "admin-service-any",
            null,
            admin,
            "/v1/admin",
            "/v1/admin/**",
            "/v2/auth/admin/**",
            "/v2/admin/**"
        ),
        paths(
            "admin-post-courses-any",
            null,
            admin,
            "/v2/post/admin/courses",
            "/v2/post/admin/courses/**"
        ),
        paths(
            "admin-v2-courses-any",
            null,
            admin,
            "/v2/admin/courses",
            "/v2/admin/courses/**"
        ),
        paths("admin-online-judge-any", null, admin, "/v2/online-judge/admin/**"),
        paths(
            "user-course-post-service-get",
            HttpMethod.GET,
            userOrganizerAdmin,
            "/v1/courses",
            "/v1/courses/**",
            "/v2/post/courses",
            "/v2/post/courses/**"
        ),
        paths(
            "user-native-course-get",
            HttpMethod.GET,
            userOrganizerAdmin,
            "/v2/courses",
            "/v2/courses/**",
            "/v2/assignments/*/course"
        ),
        paths(
            "user-submission-problem-any",
            null,
            userOrganizerAdmin,
            "/v2/submissions/**",
            "/v2/problems/**"
        ),
        paths(
            "user-report-any",
            null,
            userOrganizerAdmin,
            "/v1/report",
            "/v1/report/**",
            "/v2/report",
            "/v2/report/**"
        ),
        paths(
            "organizer-legacy-drafts-get",
            HttpMethod.GET,
            organizerAdmin,
            "/v1/posts/drafts",
            "/v1/posts/drafts/**",
            "/v2/post/drafts",
            "/v2/post/drafts/**"
        ),
        paths(
            "organizer-native-drafts-get",
            HttpMethod.GET,
            organizerAdmin,
            "/v2/posts/drafts",
            "/v2/posts/drafts/**",
            "/v2/blogs/drafts",
            "/v2/blogs/drafts/**",
            "/v2/lectures/drafts",
            "/v2/lectures/drafts/**"
        ),
        paths(
            "organizer-native-own-content-get",
            HttpMethod.GET,
            organizerAdmin,
            "/v2/posts/me",
            "/v2/posts/scheduled/me",
            "/v2/blogs/me",
            "/v2/blogs/scheduled/me",
            "/v2/lectures/me",
            "/v2/lectures/scheduled/me"
        ),
        paths(
            "user-native-content-get",
            HttpMethod.GET,
            userOrganizerAdmin,
            "/v2/posts",
            "/v2/posts/*",
            "/v2/lectures",
            "/v2/lectures/*"
        ),
        paths("public-native-blogs-get", HttpMethod.GET, AccessRequirement.PermitAll, "/v2/blogs", "/v2/blogs/*"),
        paths(
            "public-legacy-posts-get",
            HttpMethod.GET,
            AccessRequirement.PermitAll,
            "/v1/posts",
            "/v1/posts/*",
            "/v2/post",
            "/v2/post/*"
        ),
        paths(
            "organizer-content-create-post",
            HttpMethod.POST,
            organizerAdmin,
            "/v1/posts",
            "/v2/post",
            "/v2/posts",
            "/v2/blogs",
            "/v2/lectures"
        ),
        paths(
            "organizer-content-update-patch",
            HttpMethod.PATCH,
            organizerAdmin,
            "/v1/posts/*",
            "/v2/post/*",
            "/v2/posts/*",
            "/v2/blogs/*",
            "/v2/lectures/*"
        ),
        paths(
            "organizer-content-delete",
            HttpMethod.DELETE,
            organizerAdmin,
            "/v1/posts/*",
            "/v2/post/*",
            "/v2/posts/*",
            "/v2/blogs/*",
            "/v2/lectures/*"
        ),
        paths(
            "organizer-collaborator-add-post",
            HttpMethod.POST,
            organizerAdmin,
            "/v2/posts/*/collaborators"
        ),
        paths(
            "organizer-image-upload-post",
            HttpMethod.POST,
            organizerAdmin,
            "/v1/posts/images",
            "/v2/post/images",
            "/v2/post/images/**",
            "/v2/posts/images"
        ),
        anyExchange("fallback-authenticated", AccessRequirement.Authenticated)
    )

    fun validate() {
        require(rules.isNotEmpty()) { "Access rules must not be empty" }
        require(rules.all { it.id.matches(ruleIdPattern) }) { "Access rule IDs must use kebab-case" }
        require(rules.map { it.id }.toSet().size == rules.size) { "Access rule IDs must be unique" }

        val fallbackRules = rules.filter { it.matcher is AccessMatcherContract.AnyExchange }
        require(fallbackRules.size == 1) { "Access catalog must have exactly one any-exchange rule" }
        require(rules.last() == fallbackRules.single()) { "Any-exchange rule must be last" }
        require(fallbackRules.single().requirement == AccessRequirement.Authenticated) {
            "Any-exchange rule must require authentication"
        }

        val parser = PathPatternParser.defaultInstance
        val methodPathKeys = rules.flatMap { rule ->
            when (val matcher = rule.matcher) {
                AccessMatcherContract.AnyExchange -> emptyList()
                is AccessMatcherContract.Paths -> matcher.paths.map { path ->
                    parser.parse(path)
                    matcher.method to path
                }
            }
        }
        require(methodPathKeys.size == methodPathKeys.toSet().size) {
            "Access method/path declarations must be unique"
        }
    }

    private fun paths(
        id: String,
        method: HttpMethod?,
        requirement: AccessRequirement,
        vararg paths: String
    ): AccessRuleContract {
        return AccessRuleContract(
            id = id,
            matcher = AccessMatcherContract.Paths(method, paths.toList()),
            requirement = requirement
        )
    }

    private fun anyExchange(id: String, requirement: AccessRequirement): AccessRuleContract {
        return AccessRuleContract(id, AccessMatcherContract.AnyExchange, requirement)
    }

    private val ruleIdPattern = Regex("[a-z][a-z0-9]*(?:-[a-z0-9]+)*")
}
