package com.aandi.gateway.availability

import org.springframework.beans.factory.annotation.Qualifier
import org.springframework.data.redis.core.ReactiveRedisTemplate
import org.springframework.stereotype.Component
import reactor.core.publisher.Mono

private const val AVAILABILITY_HASH_KEY = "gateway:service-availability"

@Component
class BackendServiceAvailabilityRepository(
    @Qualifier("reactiveRedisTemplate")
    private val redisTemplate: ReactiveRedisTemplate<String, String>
) {

    fun isEnabled(service: BackendService): Mono<Boolean> {
        return redisTemplate.opsForHash<String, String>()
            .get(AVAILABILITY_HASH_KEY, service.name)
            .map { it.equals("true", ignoreCase = true) }
            .defaultIfEmpty(true)
            .onErrorReturn(true)
    }

    fun snapshot(): Mono<Map<BackendService, Boolean>> {
        return redisTemplate.opsForHash<String, String>()
            .entries(AVAILABILITY_HASH_KEY)
            .collectMap({ entry -> entry.key }, { entry -> entry.value })
            .map { stored ->
                BackendService.entries.associateWith { service ->
                    stored[service.name]?.equals("true", ignoreCase = true) ?: true
                }
            }
            .onErrorReturn(BackendService.entries.associateWith { true })
    }

    fun setEnabled(service: BackendService, enabled: Boolean): Mono<Boolean> {
        return redisTemplate.opsForHash<String, String>()
            .put(AVAILABILITY_HASH_KEY, service.name, enabled.toString())
    }
}
