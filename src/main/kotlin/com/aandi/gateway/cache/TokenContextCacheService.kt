package com.aandi.gateway.cache

import org.springframework.security.core.Authentication
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken
import org.springframework.stereotype.Service
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets
import java.security.MessageDigest

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
    private val cacheRepository: TokenContextCacheRepository,
    private val payloadFactory: TokenContextPayloadFactory
) : TokenContextResolver {

    override fun resolve(authentication: Authentication): Mono<TokenContextResolution> {
        val key = cacheKey(authentication)
        return cacheRepository.get(key)
            .map { cached -> TokenContextResolution(payloadJson = cached, cacheHit = true) }
            .switchIfEmpty(Mono.defer {
                val payloadJson = payloadFactory.build(authentication)
                cacheRepository.put(key, payloadJson)
                    .thenReturn(TokenContextResolution(payloadJson = payloadJson, cacheHit = false))
            })
            .onErrorResume {
                // Keep request path alive when Redis is unavailable.
                Mono.just(
                    TokenContextResolution(
                        payloadJson = payloadFactory.build(authentication),
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

    private fun sha256(value: String): String {
        val bytes = MessageDigest.getInstance("SHA-256")
            .digest(value.toByteArray(StandardCharsets.UTF_8))
        return bytes.joinToString("") { "%02x".format(it) }
    }
}
