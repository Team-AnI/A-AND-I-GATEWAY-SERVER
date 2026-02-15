package com.aandi.gateway.cache

import org.springframework.security.core.Authentication
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken
import org.springframework.stereotype.Component
import java.time.Instant
import tools.jackson.databind.ObjectMapper

@Component
class TokenContextPayloadFactory(
    private val objectMapper: ObjectMapper
) {

    fun build(authentication: Authentication): String {
        val payload = mapOf(
            "subject" to subjectOf(authentication),
            "roles" to authentication.authorities.map { it.authority },
            "cachedAt" to Instant.now().toString()
        )
        return objectMapper.writeValueAsString(payload)
    }

    private fun subjectOf(authentication: Authentication): String {
        if (authentication is JwtAuthenticationToken) {
            return authentication.token.subject ?: authentication.name
        }
        return authentication.name
    }
}
