package com.aandi.gateway.security

import org.springframework.boot.context.properties.ConfigurationProperties
import org.springframework.util.unit.DataSize

@ConfigurationProperties(prefix = "app.security.policy")
data class SecurityPolicyProperties(
    val enforceHttps: Boolean = false,
    val allowedHosts: Set<String> = emptySet(),
    val allowPrivateIpHost: Boolean = true,
    val enforceMethodPathAllowlist: Boolean = true,
    val enforceJsonContentType: Boolean = true,
    val prevalidateRefreshTokenType: Boolean = true,
    val maxRequestBodySize: DataSize = DataSize.ofMegabytes(2)
)
