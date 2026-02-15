package com.aandi.gateway.cache

import org.springframework.beans.factory.annotation.Qualifier
import org.springframework.data.redis.core.ReactiveRedisTemplate
import org.springframework.stereotype.Component
import reactor.core.publisher.Flux
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets
import java.security.MessageDigest

private const val TOKEN_CACHE_KEY_PREFIX = "cache:token"
private const val TOKEN_CACHE_INDEX_PREFIX = "cache:token-index"

@Component
class TokenContextCacheRepository(
    @Qualifier("reactiveRedisTemplate")
    private val redisTemplate: ReactiveRedisTemplate<String, String>,
    private val properties: TokenCacheProperties
) {

    fun get(key: String): Mono<String> {
        return redisTemplate.opsForValue().get(key)
    }

    fun put(key: String, payload: String): Mono<Boolean> {
        return redisTemplate.opsForValue().set(key, payload, properties.ttl)
    }

    fun tokenKey(subject: String, tokenValue: String): String {
        return "$TOKEN_CACHE_KEY_PREFIX:$subject:${sha256(tokenValue)}"
    }

    fun principalKey(principalName: String): String {
        return "$TOKEN_CACHE_KEY_PREFIX:${sha256(principalName)}"
    }

    fun addSubjectIndex(indexKey: String, tokenKey: String): Mono<Boolean> {
        return redisTemplate.opsForSet().add(indexKey, tokenKey)
            .then(redisTemplate.expire(indexKey, properties.ttl))
    }

    fun subjectIndexKey(subject: String): String {
        return "$TOKEN_CACHE_INDEX_PREFIX:$subject"
    }

    fun evictBySubjectIndex(indexKey: String): Mono<Long> {
        return redisTemplate.opsForSet().members(indexKey)
            .collectList()
            .flatMap { keys ->
                val deleteTargets = mutableListOf<String>()
                deleteTargets.add(indexKey)
                deleteTargets.addAll(keys)
                return@flatMap redisTemplate.delete(Flux.fromIterable(deleteTargets))
            }
            .onErrorReturn(0)
    }

    private fun sha256(value: String): String {
        val bytes = MessageDigest.getInstance("SHA-256")
            .digest(value.toByteArray(StandardCharsets.UTF_8))
        return bytes.joinToString("") { "%02x".format(it) }
    }
}
