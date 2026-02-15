package com.aandi.gateway.cache

import org.springframework.beans.factory.annotation.Qualifier
import org.springframework.data.redis.core.ReactiveRedisTemplate
import org.springframework.stereotype.Component
import reactor.core.publisher.Mono

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
}
