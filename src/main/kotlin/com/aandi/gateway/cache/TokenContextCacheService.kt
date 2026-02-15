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
        val cacheContext = cacheContext(authentication)
        return resolveFromCache(cacheContext)
            .switchIfEmpty(Mono.defer { storeAndResolve(cacheContext, authentication) })
            .onErrorResume {
                fallbackResolve(authentication)
            }
    }

    private fun resolveFromCache(context: CacheContext): Mono<TokenContextResolution> {
        return cacheRepository.get(context.tokenKey)
            .map { cached -> TokenContextResolution(payloadJson = cached, cacheHit = true) }
    }

    private fun storeAndResolve(
        context: CacheContext,
        authentication: Authentication
    ): Mono<TokenContextResolution> {
        val payloadJson = payloadFactory.build(authentication)
        return cacheRepository.put(context.tokenKey, payloadJson)
            .then(cacheRepository.addSubjectIndex(context.subjectIndexKey, context.tokenKey))
            .thenReturn(TokenContextResolution(payloadJson = payloadJson, cacheHit = false))
    }

    private fun fallbackResolve(authentication: Authentication): Mono<TokenContextResolution> {
        // Keep request path alive when Redis is unavailable.
        return Mono.just(
            TokenContextResolution(
                payloadJson = payloadFactory.build(authentication),
                cacheHit = false
            )
        )
    }

    private fun cacheContext(authentication: Authentication): CacheContext {
        val subject = subjectOf(authentication)
        return CacheContext(
            tokenKey = tokenKey(authentication, subject),
            subjectIndexKey = cacheRepository.subjectIndexKey(subject)
        )
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

data class CacheContext(
    val tokenKey: String,
    val subjectIndexKey: String
)
