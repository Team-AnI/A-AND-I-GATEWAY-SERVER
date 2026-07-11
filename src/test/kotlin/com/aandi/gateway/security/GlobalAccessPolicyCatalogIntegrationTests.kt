package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.context.ApplicationContext
import org.springframework.http.HttpMethod
import org.springframework.http.HttpStatus
import org.springframework.security.oauth2.jwt.ReactiveJwtDecoder
import org.springframework.security.web.server.SecurityWebFilterChain
import org.springframework.security.web.server.WebFilterChainProxy
import kotlin.test.assertEquals
import kotlin.test.assertTrue

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
class GlobalAccessPolicyCatalogIntegrationTests(
    @Autowired private val springSecurity: WebFilterChainProxy
) {
    private val securityProbe = SecurityChainProbe(springSecurity)

    @Test
    fun `global access catalog matches the live security chain for all declared paths`() {
        val witnesses = declaredPathWitnesses()

        assertEquals(105, witnesses.size)
        witnesses.forEach(::assertLiveDecision)
    }

    @Test
    fun `double wildcard roots match the live security chain`() {
        val rootWitnesses = GlobalAccessPolicyCatalog.rules.flatMap { rule ->
            val matcher = rule.matcher as? AccessMatcherContract.Paths ?: return@flatMap emptyList()
            matcher.paths
                .filter { it.endsWith("/**") }
                .map { pattern ->
                    AccessWitness(
                        sourceRuleId = rule.id,
                        declaredPattern = pattern,
                        method = matcher.method ?: HttpMethod.GET,
                        path = pattern.removeSuffix("/**").ifEmpty { "/" }
                    )
                }
        }

        assertEquals(29, rootWitnesses.size)
        rootWitnesses.forEach(::assertLiveDecision)
    }

    @Test
    fun `method boundaries match the live security chain`() {
        val pathRules = GlobalAccessPolicyCatalog.rules.mapNotNull { rule ->
            val matcher = rule.matcher as? AccessMatcherContract.Paths ?: return@mapNotNull null
            rule to matcher
        }
        val methodMismatchWitnesses = pathRules
            .filter { (_, matcher) -> matcher.method != null }
            .map { (rule, matcher) ->
                val declaredMethod = requireNotNull(matcher.method)
                AccessWitness(
                    sourceRuleId = rule.id,
                    declaredPattern = matcher.paths.first(),
                    method = alternate(declaredMethod),
                    path = materialize(matcher.paths.first())
                )
            }
        val methodlessWitnesses = pathRules
            .filter { (_, matcher) -> matcher.method == null }
            .flatMap { (rule, matcher) ->
                listOf(HttpMethod.GET, HttpMethod.PATCH).map { method ->
                    AccessWitness(
                        sourceRuleId = rule.id,
                        declaredPattern = matcher.paths.first(),
                        method = method,
                        path = materialize(matcher.paths.first())
                    )
                }
            }

        assertEquals(31, methodMismatchWitnesses.size)
        assertEquals(12, methodlessWitnesses.size)
        (methodMismatchWitnesses + methodlessWitnesses).forEach(::assertLiveDecision)
    }

    private fun declaredPathWitnesses(): List<AccessWitness> {
        return GlobalAccessPolicyCatalog.rules.flatMap { rule ->
            val matcher = rule.matcher as? AccessMatcherContract.Paths ?: return@flatMap emptyList()
            matcher.paths.map { pattern ->
                AccessWitness(
                    sourceRuleId = rule.id,
                    declaredPattern = pattern,
                    method = matcher.method ?: HttpMethod.GET,
                    path = materialize(pattern)
                )
            }
        }
    }

    private fun assertLiveDecision(witness: AccessWitness) {
        val decision = GlobalAccessPolicyCatalog.evaluate(witness.method, witness.path)
        securityProbe.assertAccess(
            witness.method,
            witness.path,
            decision.requirement,
            "${witness.method} ${witness.path} from ${witness.sourceRuleId}:${witness.declaredPattern} " +
                "won by ${decision.winningRuleId}"
        )
    }

    private fun materialize(pattern: String): String {
        return pattern.replace("**", "value/deep").replace("*", "value")
    }

    private fun alternate(method: HttpMethod): HttpMethod {
        return if (method == HttpMethod.GET) HttpMethod.POST else HttpMethod.GET
    }

    private data class AccessWitness(
        val sourceRuleId: String,
        val declaredPattern: String,
        val method: HttpMethod,
        val path: String
    )
}

@SpringBootTest(
    properties = [
        "POST_SERVICE_URI=http://localhost:8084",
        "AUTH_SERVICE_URI=http://localhost:9000",
        "ONLINE_JUDGE_SERVICE_URI=http://localhost:8080",
        "app.security.internal-event-token=test-internal-token",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!",
        "gateway.auth.enabled=false",
        "app.security.policy.enforce-https=false"
    ]
)
class PermitAllSecurityChainContractTests(
    @Autowired private val springSecurity: WebFilterChainProxy,
    @Autowired private val securityChains: List<SecurityWebFilterChain>,
    @Autowired private val applicationContext: ApplicationContext
) {
    private val securityProbe = SecurityChainProbe(springSecurity)

    @Test
    fun `disabled authentication keeps one independent permit all chain`() {
        assertEquals(1, securityChains.size)
        assertTrue(applicationContext.getBeansOfType(ReactiveJwtDecoder::class.java).isEmpty())

        val witnesses = GlobalAccessPolicyCatalog.rules.flatMap { rule ->
            val matcher = rule.matcher as? AccessMatcherContract.Paths ?: return@flatMap emptyList()
            matcher.paths.map { pattern ->
                (matcher.method ?: HttpMethod.GET) to materialize(pattern)
            }
        } + listOf(
            HttpMethod.GET to "/not-declared",
            HttpMethod.PATCH to "/v2/admin/courses/value",
            HttpMethod.OPTIONS to "/anything"
        )

        assertEquals(108, witnesses.size)
        witnesses.forEach { (method, path) ->
            assertEquals(HttpStatus.NO_CONTENT, securityProbe.exchange(method, path), "$method $path")
        }
    }

    private fun materialize(pattern: String): String {
        return pattern.replace("**", "value/deep").replace("*", "value")
    }
}
