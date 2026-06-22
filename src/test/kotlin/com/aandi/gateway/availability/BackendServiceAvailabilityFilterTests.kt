package com.aandi.gateway.availability

import org.junit.jupiter.api.Test
import org.springframework.http.server.PathContainer
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class BackendServiceMatchingTests {

    @Test
    fun `online-judge matches both legacy and namespaced submission paths`() {
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v1/submissions")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v1/submissions/42")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v1/submissions/42/stream")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v1/problems/abc/submissions/me")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v1/admin/submissions")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v1/admin/testcases")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v2/online-judge/submissions")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v2/online-judge/admin/submissions")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v2/submissions")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v2/submissions/abc")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v2/problems/p1/submissions/me")))
        assertTrue(BackendService.ONLINE_JUDGE.matches(path("/v2/admin/testcases")))
    }

    @Test
    fun `report matches course, report, and assignment-course paths across v1 and v2`() {
        assertTrue(BackendService.REPORT.matches(path("/v1/report")))
        assertTrue(BackendService.REPORT.matches(path("/v1/report/me")))
        assertTrue(BackendService.REPORT.matches(path("/v1/admin/courses")))
        assertTrue(BackendService.REPORT.matches(path("/v1/admin/courses/abc/assignments")))
        assertTrue(BackendService.REPORT.matches(path("/v1/courses")))
        assertTrue(BackendService.REPORT.matches(path("/v1/courses/abc/weeks")))
        assertTrue(BackendService.REPORT.matches(path("/v2/report")))
        assertTrue(BackendService.REPORT.matches(path("/v2/report/me")))
        assertTrue(BackendService.REPORT.matches(path("/v2/admin/courses")))
        assertTrue(BackendService.REPORT.matches(path("/v2/admin/courses/abc/assignments/1")))
        assertTrue(BackendService.REPORT.matches(path("/v2/courses/abc")))
        assertTrue(BackendService.REPORT.matches(path("/v2/assignments/123/course")))
        assertTrue(BackendService.REPORT.matches(path("/v2/post/admin/courses/abc")))
        assertTrue(BackendService.REPORT.matches(path("/v2/post/courses/abc")))
    }

    @Test
    fun `unrelated paths do not match either service`() {
        val unrelated = listOf(
            "/v2/auth/login",
            "/v1/me",
            "/v2/me",
            "/v1/admin/users",
            "/v2/admin/users",
            "/v2/admin/service-availability",
            "/v2/posts",
            "/v2/blogs/123",
            "/v2/lectures",
            "/actuator/health"
        )
        for (p in unrelated) {
            assertFalse(
                BackendService.ONLINE_JUDGE.matches(path(p)),
                "ONLINE_JUDGE should not match $p"
            )
            assertFalse(
                BackendService.REPORT.matches(path(p)),
                "REPORT should not match $p"
            )
        }
    }

    @Test
    fun `fromIdentifier resolves case-insensitively and rejects blanks`() {
        assertEquals(BackendService.ONLINE_JUDGE, BackendService.fromIdentifier("ONLINE_JUDGE"))
        assertEquals(BackendService.ONLINE_JUDGE, BackendService.fromIdentifier("online_judge"))
        assertEquals(BackendService.REPORT, BackendService.fromIdentifier(" report "))
        assertNull(BackendService.fromIdentifier(""))
        assertNull(BackendService.fromIdentifier("   "))
        assertNull(BackendService.fromIdentifier(null))
        assertNull(BackendService.fromIdentifier("post"))
    }

    private fun path(value: String): PathContainer = PathContainer.parsePath(value)
}
