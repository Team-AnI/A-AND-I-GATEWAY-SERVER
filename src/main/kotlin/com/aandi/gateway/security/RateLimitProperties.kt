package com.aandi.gateway.security

import org.springframework.boot.context.properties.ConfigurationProperties

@ConfigurationProperties(prefix = "app.security.rate-limit")
data class RateLimitProperties(
    val enabled: Boolean = true,
    val loginPerMinute: Int = 10,
    val refreshPerMinute: Int = 30,
    val logoutPerMinute: Int = 30
)
