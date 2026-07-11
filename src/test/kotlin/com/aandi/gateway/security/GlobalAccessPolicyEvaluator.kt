package com.aandi.gateway.security

import org.springframework.http.HttpMethod
import org.springframework.http.server.PathContainer
import org.springframework.web.util.pattern.PathPattern
import org.springframework.web.util.pattern.PathPatternParser

internal data class AccessDecision(
    val winningRuleId: String,
    val requirement: AccessRequirement
)

internal fun GlobalAccessPolicyCatalog.evaluate(method: HttpMethod?, path: String): AccessDecision {
    return GlobalAccessPolicyEvaluator.evaluate(method, PathContainer.parsePath(path))
}

private object GlobalAccessPolicyEvaluator {
    private val compiledRules = GlobalAccessPolicyCatalog.rules.map { rule ->
        val patterns = when (val matcher = rule.matcher) {
            AccessMatcherContract.AnyExchange -> emptyList()
            is AccessMatcherContract.Paths -> matcher.paths.map(PathPatternParser.defaultInstance::parse)
        }
        CompiledRule(rule, patterns)
    }

    fun evaluate(method: HttpMethod?, path: PathContainer): AccessDecision {
        val rule = requireNotNull(compiledRules.firstOrNull { it.matches(method, path) }) {
            "Access catalog must end with an any-exchange rule"
        }.rule
        return AccessDecision(rule.id, rule.requirement)
    }

    private data class CompiledRule(
        val rule: AccessRuleContract,
        val patterns: List<PathPattern>
    ) {
        fun matches(method: HttpMethod?, path: PathContainer): Boolean {
            return when (val matcher = rule.matcher) {
                AccessMatcherContract.AnyExchange -> true
                is AccessMatcherContract.Paths ->
                    (matcher.method == null || matcher.method == method) && patterns.any { it.matches(path) }
            }
        }
    }
}
