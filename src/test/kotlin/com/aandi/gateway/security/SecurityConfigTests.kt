package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.cloud.gateway.route.RouteDefinition
import org.springframework.cloud.gateway.route.RouteDefinitionLocator
import org.springframework.context.ApplicationContext
import org.springframework.http.HttpMethod
import org.springframework.http.MediaType
import org.springframework.security.core.authority.SimpleGrantedAuthority
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.mockJwt
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.springSecurity
import org.springframework.test.web.reactive.server.WebTestClient
import org.springframework.web.reactive.function.BodyInserters
import kotlin.test.assertEquals
import kotlin.test.assertNotEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

@SpringBootTest(
    properties = [
        "POST_SERVICE_URI=http://localhost:8084",
        "AUTH_SERVICE_URI=http://localhost:9000",
        "app.security.internal-event-token=test-internal-token",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!",
        "app.security.policy.enforce-https=false"
    ]
)
class SecurityConfigTests(
    @Autowired private val applicationContext: ApplicationContext,
    @Autowired private val routeDefinitionLocator: RouteDefinitionLocator
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
    fun `auth login endpoint is public`() {
        webTestClient.post()
            .uri("/v1/auth/login")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"username":"demo","password":"demo"}""")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
            }
    }

    @Test
    fun `activate endpoint is public`() {
        webTestClient.post()
            .uri("/activate")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"token":"invite-token","password":"new-password-1234"}""")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
                assertNotEquals(404, it)
            }
    }

    @Test
    fun `me endpoint requires authentication`() {
        webTestClient.get()
            .uri("/v1/me")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `me endpoint unauthorized response includes cors header`() {
        webTestClient.get()
            .uri("/v1/me")
            .header("Origin", "https://aandiclub.com")
            .exchange()
            .expectStatus()
            .value { status ->
                assertTrue(status == 401 || status == 403)
            }
            .expectHeader()
            .valueEquals("Access-Control-Allow-Origin", "https://aandiclub.com")
    }

    @Test
    fun `me patch endpoint requires authentication`() {
        webTestClient.patch()
            .uri("/v1/me")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"username":"updated-user"}""")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `me post endpoint requires authentication`() {
        webTestClient.post()
            .uri("/v1/me")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"displayName":"new-user"}""")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `me post multipart endpoint requires authentication`() {
        webTestClient.post()
            .uri("/v1/me")
            .contentType(MediaType.MULTIPART_FORM_DATA)
            .body(BodyInserters.fromMultipartData("nickname", "new-user"))
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `posts create multipart endpoint requires authentication`() {
        webTestClient.post()
            .uri("/v1/posts")
            .contentType(MediaType.MULTIPART_FORM_DATA)
            .body(BodyInserters.fromMultipartData("title", "t").with("content", "c"))
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `post images multipart endpoint requires authentication`() {
        webTestClient.post()
            .uri("/v1/posts/images")
            .contentType(MediaType.MULTIPART_FORM_DATA)
            .body(BodyInserters.fromMultipartData("file", "dummy"))
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `post patch multipart endpoint requires authentication`() {
        webTestClient.patch()
            .uri("/v1/posts/123")
            .contentType(MediaType.MULTIPART_FORM_DATA)
            .body(BodyInserters.fromMultipartData("title", "t").with("content", "c"))
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `me password endpoint requires authentication`() {
        webTestClient.post()
            .uri("/v1/me/password")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"currentPassword":"old-password-1234","newPassword":"new-password-1234"}""")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `admin endpoint is forbidden for non admin role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .get()
            .uri("/v1/admin/ping")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `admin reset password endpoint is forbidden for non admin role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .post()
            .uri("/v1/admin/users/11111111-1111-1111-1111-111111111111/reset-password")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("{}")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `admin role patch endpoint is forbidden for non admin role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .patch()
            .uri("/v1/admin/users/role")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"userId":"11111111-1111-1111-1111-111111111111","role":"ORGANIZER"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `admin users delete endpoint is forbidden for non admin role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .method(HttpMethod.DELETE)
            .uri("/v1/admin/users")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"userId":"11111111-1111-1111-1111-111111111111"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `admin invite mail endpoint is forbidden for non admin role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .post()
            .uri("/v1/admin/invite-mail")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"email":"new_member@aandi.club","role":"USER"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `legacy admin invite mail endpoint is forbidden for non admin role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .post()
            .uri("/v2/auth/admin/invite-mail")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"email":"new_member@aandi.club","role":"USER"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `posts list is public`() {
        webTestClient.get()
            .uri("/v1/posts")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
            }
    }

    @Test
    fun `drafts subpath endpoint is allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v1/posts/drafts/me")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `drafts subpath endpoint is forbidden for user role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .get()
            .uri("/v1/posts/drafts/me")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `drafts root endpoint is allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v1/posts/drafts")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `drafts root endpoint is forbidden for user role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .get()
            .uri("/v1/posts/drafts")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `drafts me route does not rewrite to drafts root`() {
        val draftsMeRoute = routeById("post-service-v1-posts-drafts-me")
        val hasSetPath = draftsMeRoute.filters.any { it.name == "SetPath" }

        assertTrue(!hasSetPath, "drafts/me route must not rewrite to drafts root")
    }

    @Test
    fun `drafts root route remains separate`() {
        val draftsRoute = routeById("post-service-v1-posts-drafts")
        val pathPredicate = draftsRoute.predicates.firstOrNull { it.name == "Path" }
        val methodPredicate = draftsRoute.predicates.firstOrNull { it.name == "Method" }

        assertNotNull(pathPredicate, "drafts root route should have path predicate")
        assertNotNull(methodPredicate, "drafts root route should have method predicate")
        assertTrue(pathPredicate.args.values.contains("/v1/posts/drafts"))
        assertTrue(methodPredicate.args.values.contains("GET"))
    }

    @Test
    fun `legacy drafts me route maps to drafts me backend path`() {
        val legacyDraftsMeRoute = routeById("post-service-drafts-me")
        val setPathFilter = legacyDraftsMeRoute.filters.firstOrNull { it.name == "SetPath" }

        assertNotNull(setPathFilter, "legacy drafts/me route should set backend path")
        assertEquals("/v1/posts/drafts/me", setPathFilter.args.values.firstOrNull())
    }

    @Test
    fun `post create requires organizer or admin`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .post()
            .uri("/v1/posts")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"title":"t","content":"c"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `post delete allows organizer or admin`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_ORGANIZER")))
            .delete()
            .uri("/v1/posts/123")
            .exchange()
            .expectStatus()
            .value { assertNotEquals(403, it) }
    }

    @Test
    fun `report endpoint is allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v2/report")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `v1 report endpoint is allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v1/report")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `course query endpoint is allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v2/post/courses")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `v1 course admin enrollments endpoint is allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v1/admin/courses/back-basic/enrollments")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `v1 assignment to course query endpoint is allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v1/courses/assignments/11111111-1111-1111-1111-111111111111/course")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `legacy post assignment to course query endpoint remains allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v2/post/courses/assignments/11111111-1111-1111-1111-111111111111/course")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `users endpoint is allowlisted and requires authentication`() {
        webTestClient.get()
            .uri("/v1/users/123")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `course admin endpoint is forbidden for non admin role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .post()
            .uri("/v2/post/admin/courses")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"title":"course","slug":"course-slug"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `internal invalidation endpoint is forbidden without internal token`() {
        webTestClient.post()
            .uri("/internal/v1/cache/invalidation")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"eventType":"LOGOUT","subject":"user-1"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `internal invalidation endpoint accepts valid internal token`() {
        webTestClient.post()
            .uri("/internal/v1/cache/invalidation")
            .header("X-Internal-Token", "test-internal-token")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"eventType":"LOGOUT","subject":"user-1"}""")
            .exchange()
            .expectStatus()
            .isAccepted
    }

    private fun routeById(routeId: String): RouteDefinition {
        val definitions = routeDefinitionLocator.routeDefinitions.collectList().block().orEmpty()
        return definitions.firstOrNull { it.id == routeId }
            ?: error("Missing route definition: $routeId")
    }
}
