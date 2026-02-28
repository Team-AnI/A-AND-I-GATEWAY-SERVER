package com.aandi.gateway.security

import org.springframework.boot.context.properties.ConfigurationProperties

@ConfigurationProperties(prefix = "app.security.policy")
data class SecurityPolicyProperties(
    val enforceHttps: Boolean = false,
    val allowedHosts: Set<String> = emptySet(),
    val allowPrivateIpHost: Boolean = true,
    val enforceMethodPathAllowlist: Boolean = true,
    val enforceJsonContentType: Boolean = true,
    val prevalidateRefreshTokenType: Boolean = true
)
