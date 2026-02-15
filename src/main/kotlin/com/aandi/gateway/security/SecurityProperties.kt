package com.aandi.gateway.security

import org.springframework.boot.context.properties.ConfigurationProperties

@ConfigurationProperties(prefix = "app.security")
data class SecurityProperties(
    val requiredAudience: String = "aandi-gateway",
    val internalEventToken: String
)
