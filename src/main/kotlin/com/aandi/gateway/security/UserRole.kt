package com.aandi.gateway.security

import org.springframework.security.core.GrantedAuthority
import org.springframework.security.core.authority.SimpleGrantedAuthority

enum class UserRole {
    USER,
    ORGANIZER,
    ADMIN;

    fun grantedAuthorities(): List<GrantedAuthority> {
        return when (this) {
            USER -> listOf(SimpleGrantedAuthority("ROLE_USER"))
            ORGANIZER -> listOf(
                SimpleGrantedAuthority("ROLE_ORGANIZER"),
                SimpleGrantedAuthority("ROLE_USER")
            )
            ADMIN -> listOf(
                SimpleGrantedAuthority("ROLE_ADMIN"),
                SimpleGrantedAuthority("ROLE_ORGANIZER"),
                SimpleGrantedAuthority("ROLE_USER")
            )
        }
    }

    companion object {
        fun fromClaim(raw: String?): UserRole? {
            if (raw.isNullOrBlank()) return null
            return entries.firstOrNull { it.name == raw.trim().uppercase() }
        }
    }
}
