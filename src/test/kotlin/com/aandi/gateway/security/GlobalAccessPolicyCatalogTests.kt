package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.http.HttpMethod
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.util.HexFormat
import kotlin.test.assertEquals

class GlobalAccessPolicyCatalogTests {

    @Test
    fun `global access catalog preserves the complete ordered inventory`() {
        GlobalAccessPolicyCatalog.validate()

        val rules = GlobalAccessPolicyCatalog.rules
        val pathRules = rules.filter { it.matcher is AccessMatcherContract.Paths }
        val flattenedPaths = pathRules.flatMap { rule ->
            val matcher = rule.matcher as AccessMatcherContract.Paths
            matcher.paths.map { path -> MethodPathKey(matcher.method, path) }
        }

        assertEquals(38, rules.size)
        assertEquals(37, pathRules.size)
        assertEquals(105, flattenedPaths.size)
        assertEquals(105, flattenedPaths.toSet().size)
        assertEquals(
            mapOf(
                AccessRequirement.PermitAll to RulePathCount(rules = 12, paths = 30),
                roles(UserRole.USER, UserRole.ORGANIZER, UserRole.ADMIN) to
                    RulePathCount(rules = 11, paths = 28),
                roles(UserRole.ORGANIZER, UserRole.ADMIN) to RulePathCount(rules = 9, paths = 37),
                roles(UserRole.ADMIN) to RulePathCount(rules = 5, paths = 10),
                AccessRequirement.Authenticated to RulePathCount(rules = 1, paths = 0)
            ),
            rules.groupBy { it.requirement }.mapValues { (_, groupedRules) ->
                RulePathCount(
                    rules = groupedRules.size,
                    paths = groupedRules.sumOf { rule ->
                        (rule.matcher as? AccessMatcherContract.Paths)?.paths?.size ?: 0
                    }
                )
            }
        )
        assertEquals(
            mapOf(
                HttpMethod.OPTIONS to RulePathCount(rules = 1, paths = 1),
                HttpMethod.GET to RulePathCount(rules = 17, paths = 54),
                HttpMethod.POST to RulePathCount(rules = 9, paths = 21),
                HttpMethod.PATCH to RulePathCount(rules = 3, paths = 9),
                HttpMethod.DELETE to RulePathCount(rules = 1, paths = 5),
                null to RulePathCount(rules = 6, paths = 15)
            ),
            pathRules
                .map { it.matcher as AccessMatcherContract.Paths }
                .groupBy { it.method }
                .mapValues { (_, matchers) ->
                    RulePathCount(matchers.size, matchers.sumOf { it.paths.size })
                }
        )
        assertEquals(
            "db6b95947d15605690b9844ae78ed1764928d9ea8bf97f92c237d923868d6eed",
            fingerprint(rules)
        )
    }

    @Test
    fun `global access catalog preserves first match winning rules`() {
        val permitAll = AccessRequirement.PermitAll
        val authenticated = AccessRequirement.Authenticated
        val user = roles(UserRole.USER, UserRole.ORGANIZER, UserRole.ADMIN)
        val organizer = roles(UserRole.ORGANIZER, UserRole.ADMIN)
        val admin = roles(UserRole.ADMIN)
        val cases = listOf(
            DecisionCase(HttpMethod.OPTIONS, "/v2/admin/courses/java", "preflight-options", permitAll),
            DecisionCase(HttpMethod.GET, "/v1/admin/courses", "admin-v1-courses-get", admin),
            DecisionCase(HttpMethod.PATCH, "/v1/admin/courses", "admin-service-any", admin),
            DecisionCase(HttpMethod.GET, "/v2/admin/courses", "admin-service-any", admin),
            DecisionCase(HttpMethod.PATCH, "/v2/admin/courses/java", "admin-service-any", admin),
            DecisionCase(HttpMethod.GET, "/v1/posts/drafts", "organizer-legacy-drafts-get", organizer),
            DecisionCase(HttpMethod.GET, "/v2/post/drafts/value", "organizer-legacy-drafts-get", organizer),
            DecisionCase(HttpMethod.GET, "/v2/posts/drafts", "organizer-native-drafts-get", organizer),
            DecisionCase(HttpMethod.GET, "/v2/blogs/drafts/value", "organizer-native-drafts-get", organizer),
            DecisionCase(HttpMethod.GET, "/v2/posts/me", "organizer-native-own-content-get", organizer),
            DecisionCase(HttpMethod.GET, "/v2/blogs/me", "organizer-native-own-content-get", organizer),
            DecisionCase(HttpMethod.GET, "/v2/lectures/me", "organizer-native-own-content-get", organizer),
            DecisionCase(HttpMethod.GET, "/v2/blogs/value", "public-native-blogs-get", permitAll),
            DecisionCase(HttpMethod.POST, "/v2/blogs", "organizer-content-create-post", organizer),
            DecisionCase(HttpMethod.GET, "/v2/posts/value", "user-native-content-get", user),
            DecisionCase(HttpMethod.GET, "/v2/auth/login", "fallback-authenticated", authenticated),
            DecisionCase(HttpMethod.POST, "/v2/auth/me/password", "fallback-authenticated", authenticated),
            DecisionCase(HttpMethod.GET, "/not-declared", "fallback-authenticated", authenticated)
        )

        cases.forEach { case ->
            assertEquals(
                AccessDecision(case.ruleId, case.requirement),
                GlobalAccessPolicyCatalog.evaluate(case.method, case.path),
                "${case.method} ${case.path}"
            )
        }
    }

    @Test
    fun `single and double wildcard boundaries remain distinct`() {
        val permitAll = AccessRequirement.PermitAll
        val authenticated = AccessRequirement.Authenticated
        val user = roles(UserRole.USER, UserRole.ORGANIZER, UserRole.ADMIN)
        val cases = listOf(
            DecisionCase(
                HttpMethod.GET,
                "/v2/auth/v3/api-docs",
                "public-service-openapi-get",
                permitAll
            ),
            DecisionCase(
                HttpMethod.GET,
                "/v2/auth/v3/api-docs/v1/deep",
                "public-service-openapi-get",
                permitAll
            ),
            DecisionCase(
                HttpMethod.GET,
                "/v2/auth/extra/v3/api-docs",
                "fallback-authenticated",
                authenticated
            ),
            DecisionCase(
                HttpMethod.GET,
                "/v2/assignments/value/course",
                "user-native-course-get",
                user
            ),
            DecisionCase(
                HttpMethod.GET,
                "/v2/assignments/value/deep/course",
                "fallback-authenticated",
                authenticated
            ),
            DecisionCase(
                HttpMethod.GET,
                "/v2/posts/value/deep",
                "fallback-authenticated",
                authenticated
            )
        )

        cases.forEach { case ->
            assertEquals(
                AccessDecision(case.ruleId, case.requirement),
                GlobalAccessPolicyCatalog.evaluate(case.method, case.path),
                "${case.method} ${case.path}"
            )
        }
    }

    private fun fingerprint(rules: List<AccessRuleContract>): String {
        val canonical = rules.joinToString("\n") { rule ->
            when (val matcher = rule.matcher) {
                AccessMatcherContract.AnyExchange ->
                    "${rule.id}|ANY_EXCHANGE|ANY|-|${rule.requirement.canonicalName()}"

                is AccessMatcherContract.Paths ->
                    "${rule.id}|PATH|${matcher.method?.name() ?: "ANY"}|" +
                        "${matcher.paths.joinToString(",")}|${rule.requirement.canonicalName()}"
            }
        }
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(canonical.toByteArray(StandardCharsets.UTF_8))
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
                else -> error("Unsupported role set: ${roles.map(UserRole::name).sorted()}")
            }
        }
    }

    private fun roles(vararg roles: UserRole): AccessRequirement.AnyRole {
        return AccessRequirement.AnyRole(roles.toSet())
    }

    private data class MethodPathKey(
        val method: HttpMethod?,
        val path: String
    )

    private data class RulePathCount(
        val rules: Int,
        val paths: Int
    )

    private data class DecisionCase(
        val method: HttpMethod,
        val path: String,
        val ruleId: String,
        val requirement: AccessRequirement
    )
}
