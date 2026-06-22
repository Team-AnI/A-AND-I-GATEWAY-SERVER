package com.aandi.gateway.availability

import org.springframework.http.server.PathContainer
import org.springframework.web.util.pattern.PathPattern
import org.springframework.web.util.pattern.PathPatternParser

private fun parse(vararg patterns: String): List<PathPattern> {
    val parser = PathPatternParser.defaultInstance
    return patterns.map(parser::parse)
}

enum class BackendService(val pathPatterns: List<PathPattern>) {
    ONLINE_JUDGE(
        parse(
            "/v1/submissions",
            "/v1/submissions/**",
            "/v1/problems/*/submissions/me",
            "/v1/admin/submissions",
            "/v1/admin/testcases",
            "/v2/submissions",
            "/v2/submissions/**",
            "/v2/problems/*/submissions/me",
            "/v2/admin/submissions",
            "/v2/admin/testcases",
            "/v2/online-judge/**"
        )
    ),
    REPORT(
        parse(
            "/v1/report",
            "/v1/report/**",
            "/v1/admin/courses",
            "/v1/admin/courses/**",
            "/v1/courses",
            "/v1/courses/**",
            "/v2/report",
            "/v2/report/**",
            "/v2/admin/courses",
            "/v2/admin/courses/**",
            "/v2/courses",
            "/v2/courses/**",
            "/v2/assignments/*/course",
            "/v2/post/admin/courses",
            "/v2/post/admin/courses/**",
            "/v2/post/courses",
            "/v2/post/courses/**"
        )
    );

    fun matches(path: PathContainer): Boolean {
        return pathPatterns.any { it.matches(path) }
    }

    companion object {
        fun fromIdentifier(raw: String?): BackendService? {
            if (raw.isNullOrBlank()) return null
            return entries.firstOrNull { it.name.equals(raw.trim(), ignoreCase = true) }
        }
    }
}
