package com.aandi.gateway.cache

import org.springframework.boot.context.properties.ConfigurationProperties
import java.time.Duration

@ConfigurationProperties(prefix = "app.token-cache")
data class TokenCacheProperties(
    val ttl: Duration = Duration.ofHours(24)
)
