package com.aandi.gateway.cache

import org.springframework.security.core.Authentication
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken
import org.springframework.stereotype.Service
import reactor.core.publisher.Mono

private const val UNKNOWN_SUBJECT = "unknown"

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
        val subject = subjectOf(authentication)
        val key = tokenKey(authentication, subject)
        val subjectIndexKey = cacheRepository.subjectIndexKey(subject)
        return cacheRepository.get(key)
            .map { cached -> TokenContextResolution(payloadJson = cached, cacheHit = true) }
            .switchIfEmpty(Mono.defer {
                val payloadJson = payloadFactory.build(authentication)
                cacheRepository.put(key, payloadJson)
                    .then(cacheRepository.addSubjectIndex(subjectIndexKey, key))
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

    private fun tokenKey(authentication: Authentication, subject: String): String {
        if (authentication is JwtAuthenticationToken) {
            return cacheRepository.tokenKey(subject, authentication.token.tokenValue)
        }
        return cacheRepository.principalKey(authentication.name)
    }

    private fun subjectOf(authentication: Authentication): String {
        if (authentication is JwtAuthenticationToken) {
            return authentication.token.subject ?: UNKNOWN_SUBJECT
        }
        return authentication.name
    }
}
