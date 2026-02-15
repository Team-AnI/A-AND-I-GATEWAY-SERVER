package com.aandi.gateway.cache

import org.springframework.beans.factory.annotation.Qualifier
import org.springframework.data.redis.core.ReactiveRedisTemplate
import org.springframework.stereotype.Component
import reactor.core.publisher.Flux
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets
import java.security.MessageDigest

@JvmInline
value class CacheKeyPrefix(private val value: String) {
    fun with(vararg parts: String): String {
        return (listOf(value) + parts.toList()).joinToString(":")
    }
}

data class DeleteTargetKeys(
    val values: List<String>
) {
    fun toFlux(): Flux<String> = Flux.fromIterable(values)
}

private val TOKEN_CACHE_KEY_PREFIX = CacheKeyPrefix("cache:token")
private val TOKEN_CACHE_INDEX_PREFIX = CacheKeyPrefix("cache:token-index")

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
        return TOKEN_CACHE_KEY_PREFIX.with(subject, sha256(tokenValue))
    }

    fun principalKey(principalName: String): String {
        return TOKEN_CACHE_KEY_PREFIX.with(sha256(principalName))
    }

    fun addSubjectIndex(indexKey: String, tokenKey: String): Mono<Boolean> {
        return redisTemplate.opsForSet().add(indexKey, tokenKey)
            .then(redisTemplate.expire(indexKey, properties.ttl))
    }

    fun subjectIndexKey(subject: String): String {
        return TOKEN_CACHE_INDEX_PREFIX.with(subject)
    }

    fun evictBySubjectIndex(indexKey: String): Mono<Long> {
        return redisTemplate.opsForSet().members(indexKey)
            .collectList()
            .map { memberKeys -> deleteTargets(indexKey, memberKeys) }
            .flatMap(::deleteAll)
            .onErrorReturn(0)
    }

    private fun deleteTargets(indexKey: String, memberKeys: List<String>): DeleteTargetKeys {
        val values = listOf(indexKey) + memberKeys
        return DeleteTargetKeys(values)
    }

    private fun deleteAll(targets: DeleteTargetKeys): Mono<Long> {
        return redisTemplate.delete(targets.toFlux())
    }

    private fun sha256(value: String): String {
        val bytes = MessageDigest.getInstance("SHA-256")
            .digest(value.toByteArray(StandardCharsets.UTF_8))
        return bytes.joinToString("") { "%02x".format(it) }
    }
}
