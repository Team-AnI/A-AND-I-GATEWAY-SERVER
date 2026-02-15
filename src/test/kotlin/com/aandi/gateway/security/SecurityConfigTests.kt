package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.context.ApplicationContext
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.mockJwt
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.springSecurity
import org.springframework.test.web.reactive.server.WebTestClient
import kotlin.test.assertNotEquals

@SpringBootTest
class SecurityConfigTests(
    @Autowired private val applicationContext: ApplicationContext
) {
    private val webTestClient: WebTestClient by lazy {
        WebTestClient.bindToApplicationContext(applicationContext)
            .apply(springSecurity())
            .configureClient()
            .build()
    }

    @Test
    fun `health endpoint is public`() {
        webTestClient.get()
            .uri("/actuator/health")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
            }
    }

    @Test
    fun `token context endpoint requires authentication`() {
        webTestClient.get()
            .uri("/v2/cache/token-context")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `token context endpoint accepts authenticated jwt`() {
        webTestClient.mutateWith(mockJwt())
            .get()
            .uri("/v2/cache/token-context")
            .exchange()
            .expectStatus()
            .isNotFound
    }
}
