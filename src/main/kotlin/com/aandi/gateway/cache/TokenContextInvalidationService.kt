package com.aandi.gateway.cache

import org.springframework.stereotype.Service
import reactor.core.publisher.Mono

@Service
class TokenContextInvalidationService(
    private val cacheRepository: TokenContextCacheRepository
) {

    fun invalidateOnLogout(subject: String): Mono<Long> {
        return invalidateBySubject(subject)
    }

    fun invalidateOnRoleChanged(subject: String): Mono<Long> {
        return invalidateBySubject(subject)
    }

    private fun invalidateBySubject(subject: String): Mono<Long> {
        val indexKey = cacheRepository.subjectIndexKey(subject)
        return cacheRepository.evictBySubjectIndex(indexKey)
    }
}
