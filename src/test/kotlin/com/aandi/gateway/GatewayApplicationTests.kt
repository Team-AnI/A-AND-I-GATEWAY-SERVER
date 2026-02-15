package com.aandi.gateway

import org.junit.jupiter.api.Test
import org.springframework.boot.test.context.SpringBootTest

@SpringBootTest(
	properties = [
		"app.security.internal-event-token=test-internal-token"
	]
)
class GatewayApplicationTests {

	@Test
	fun contextLoads() {
	}

}
