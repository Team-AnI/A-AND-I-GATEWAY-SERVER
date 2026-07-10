package com.aandi.gateway.security

import com.aandi.gateway.availability.BackendServiceAvailabilityFilter
import com.aandi.gateway.cache.TokenContextHeaderFilter
import com.aandi.gateway.logging.RequestResponseLoggingFilter
import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.context.ApplicationContext
import org.springframework.core.Ordered
import org.springframework.security.web.server.WebFilterChainProxy
import org.springframework.web.server.WebFilter
import kotlin.test.assertEquals
import kotlin.test.assertTrue

@SpringBootTest(
    properties = [
        "app.security.internal-event-token=test-internal-token",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!"
    ]
)
class GatewayFilterOrderContractTests(
    @Autowired private val cors: CorsResponseHeaderFilter,
    @Autowired private val logging: RequestResponseLoggingFilter,
    @Autowired private val requestPolicy: GatewayRequestPolicyFilter,
    @Autowired private val backendAvailability: BackendServiceAvailabilityFilter,
    @Autowired private val authRateLimit: AuthRateLimitFilter,
    @Autowired private val authRequestValidation: AuthRequestValidationFilter,
    @Autowired private val springSecurity: WebFilterChainProxy,
    @Autowired private val tokenContextHeader: TokenContextHeaderFilter,
    @Autowired private val authenticatedPrincipalHeader: AuthenticatedPrincipalHeaderFilter,
    @Autowired private val applicationContext: ApplicationContext
) {

    @Test
    fun `application WebFilter order values remain stable`() {
        assertEquals(
            listOf(
                Ordered.HIGHEST_PRECEDENCE,
                Ordered.HIGHEST_PRECEDENCE + 5,
                Ordered.HIGHEST_PRECEDENCE + 20,
                Ordered.HIGHEST_PRECEDENCE + 25,
                Ordered.HIGHEST_PRECEDENCE + 30,
                Ordered.HIGHEST_PRECEDENCE + 31,
                Ordered.LOWEST_PRECEDENCE - 200,
                Ordered.LOWEST_PRECEDENCE - 100
            ),
            listOf(
                cors.order,
                logging.order,
                requestPolicy.order,
                backendAvailability.order,
                authRateLimit.order,
                authRequestValidation.order,
                tokenContextHeader.order,
                authenticatedPrincipalHeader.order
            )
        )
    }

    @Test
    fun `meaningful WebFilter relationships remain stable across the security boundary`() {
        val orderedFilters = applicationContext
            .getBeanProvider(WebFilter::class.java)
            .orderedStream()
            .toList()

        assertBefore(orderedFilters, cors, logging)
        assertBefore(orderedFilters, logging, requestPolicy)
        assertBefore(orderedFilters, requestPolicy, backendAvailability)
        assertBefore(orderedFilters, authRateLimit, authRequestValidation)
        assertBefore(orderedFilters, authRequestValidation, springSecurity)
        assertBefore(orderedFilters, springSecurity, tokenContextHeader)
        assertBefore(orderedFilters, springSecurity, authenticatedPrincipalHeader)
    }

    private fun assertBefore(orderedFilters: List<WebFilter>, first: WebFilter, second: WebFilter) {
        val firstIndex = orderedFilters.indexOfFirst { it === first }
        val secondIndex = orderedFilters.indexOfFirst { it === second }
        assertTrue(firstIndex >= 0, "${first.javaClass.simpleName} is missing from the ordered WebFilter chain")
        assertTrue(secondIndex >= 0, "${second.javaClass.simpleName} is missing from the ordered WebFilter chain")
        assertTrue(
            firstIndex < secondIndex,
            "${first.javaClass.simpleName} must execute before ${second.javaClass.simpleName}"
        )
    }
}
