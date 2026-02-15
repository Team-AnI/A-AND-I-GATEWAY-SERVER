package com.aandi.gateway.cache

import org.springframework.beans.factory.annotation.Value
import org.springframework.beans.factory.annotation.Qualifier
import org.springframework.data.redis.core.ReactiveRedisTemplate
import org.springframework.security.core.Authentication
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken
import org.springframework.stereotype.Service
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.time.Duration
import java.time.Instant
import tools.jackson.databind.ObjectMapper

private const val UNKNOWN_SUBJECT = "unknown"
private const val TOKEN_CACHE_KEY_PREFIX = "cache:token"

data class TokenContextResolution(
    val payloadJson: String,
    val cacheHit: Boolean
)

interface TokenContextResolver {
    fun resolve(authentication: Authentication): Mono<TokenContextResolution>
}

@Service
class TokenContextCacheService(
    @Qualifier("reactiveRedisTemplate")
    private val redisTemplate: ReactiveRedisTemplate<String, String>,
    private val objectMapper: ObjectMapper,
    @Value("\${app.token-cache.ttl:24h}") private val cacheTtl: Duration
) : TokenContextResolver {

    override fun resolve(authentication: Authentication): Mono<TokenContextResolution> {
        val key = cacheKey(authentication)
        return redisTemplate.opsForValue().get(key)
            .map { cached -> TokenContextResolution(payloadJson = cached, cacheHit = true) }
            .switchIfEmpty(Mono.defer {
                val payloadJson = buildPayload(authentication)
                redisTemplate.opsForValue()
                    .set(key, payloadJson, cacheTtl)
                    .thenReturn(TokenContextResolution(payloadJson = payloadJson, cacheHit = false))
            })
            .onErrorResume {
                // Keep request path alive when Redis is unavailable.
                Mono.just(
                    TokenContextResolution(
                        payloadJson = buildPayload(authentication),
                        cacheHit = false
                    )
                )
            }
    }

    private fun cacheKey(authentication: Authentication): String {
        if (authentication is JwtAuthenticationToken) {
            val subject = authentication.token.subject ?: UNKNOWN_SUBJECT
            val tokenHash = sha256(authentication.token.tokenValue)
            return "$TOKEN_CACHE_KEY_PREFIX:$subject:$tokenHash"
        }
        return "$TOKEN_CACHE_KEY_PREFIX:${sha256(authentication.name)}"
    }

    private fun buildPayload(authentication: Authentication): String {
        val roles = authentication.authorities.map { it.authority }
        if (authentication is JwtAuthenticationToken) {
            return objectMapper.writeValueAsString(
                mapOf(
                    "subject" to (authentication.token.subject ?: authentication.name),
                    "roles" to roles,
                    "claims" to authentication.token.claims,
                    "cachedAt" to Instant.now().toString()
                )
            )
        }
        return objectMapper.writeValueAsString(
            mapOf(
                "subject" to authentication.name,
                "roles" to roles,
                "cachedAt" to Instant.now().toString()
            )
        )
    }

    private fun sha256(value: String): String {
        val bytes = MessageDigest.getInstance("SHA-256")
            .digest(value.toByteArray(StandardCharsets.UTF_8))
        return bytes.joinToString("") { "%02x".format(it) }
    }
}
