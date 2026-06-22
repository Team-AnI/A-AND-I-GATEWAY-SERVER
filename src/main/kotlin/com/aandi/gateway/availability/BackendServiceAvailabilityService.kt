package com.aandi.gateway.availability

import org.springframework.stereotype.Service
import reactor.core.publisher.Mono

data class ServiceAvailabilityState(
    val service: BackendService,
    val enabled: Boolean
)

@Service
class BackendServiceAvailabilityService(
    private val repository: BackendServiceAvailabilityRepository
) {

    fun isEnabled(service: BackendService): Mono<Boolean> {
        return repository.isEnabled(service)
    }

    fun listAll(): Mono<List<ServiceAvailabilityState>> {
        return repository.snapshot().map { snapshot ->
            BackendService.entries.map { ServiceAvailabilityState(it, snapshot[it] ?: true) }
        }
    }

    fun setEnabled(service: BackendService, enabled: Boolean): Mono<ServiceAvailabilityState> {
        return repository.setEnabled(service, enabled)
            .thenReturn(ServiceAvailabilityState(service, enabled))
    }
}
