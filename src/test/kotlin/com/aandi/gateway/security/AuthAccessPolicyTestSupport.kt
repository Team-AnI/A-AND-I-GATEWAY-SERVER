package com.aandi.gateway.security

internal fun EndpointPolicyContract.witnessPath(): String {
    return path
        .replace(pathVariable, "value")
        .replace("**", "value")
}

private val pathVariable = Regex("""\{[^/{}]+}""")
