package com.aandi.gateway.logging

import org.springframework.boot.context.properties.ConfigurationProperties

@ConfigurationProperties("aandi.logging")
data class ApiLoggingProperties(
    val env: String = "local",
    val service: ServiceProperties = ServiceProperties()
) {
    data class ServiceProperties(
        val name: String = "gateway",
        val domainCode: Int = 1,
        val version: String = "unknown",
        val instanceId: String = "unknown"
    )
}
