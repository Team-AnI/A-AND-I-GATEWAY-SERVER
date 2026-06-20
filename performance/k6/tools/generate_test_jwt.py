#!/usr/bin/env python3
import argparse
import base64
import hashlib
import hmac
import json
import time
import uuid


def b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")


def encode_json(value: dict) -> str:
    return b64url(json.dumps(value, separators=(",", ":"), sort_keys=True).encode("utf-8"))


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate a local-only HS256 JWT for Gateway performance tests.")
    parser.add_argument("--secret", required=True)
    parser.add_argument("--issuer", required=True)
    parser.add_argument("--audience", required=True)
    parser.add_argument("--role", choices=["USER", "ADMIN"], required=True)
    args = parser.parse_args()

    if len(args.secret.encode("utf-8")) < 32:
        raise SystemExit("--secret must be at least 32 bytes")

    now = int(time.time())
    header = {
        "alg": "HS256",
        "typ": "JWT",
    }
    payload = {
        "iss": args.issuer,
        "aud": [args.audience],
        "sub": str(uuid.uuid4()),
        "role": args.role,
        "token_type": "ACCESS",
        "jti": str(uuid.uuid4()),
        "iat": now,
        "nbf": now - 1,
        "exp": now + 3600,
    }

    signing_input = f"{encode_json(header)}.{encode_json(payload)}"
    signature = hmac.new(args.secret.encode("utf-8"), signing_input.encode("ascii"), hashlib.sha256).digest()
    print(f"{signing_input}.{b64url(signature)}")


if __name__ == "__main__":
    main()
